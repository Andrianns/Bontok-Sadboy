package main

import (
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Article struct {
	Title     string    `json:"title"`
	Source    string    `json:"source"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
}
type Topic struct {
	ID uint32 `json:"id"`
	Article
	Category string    `json:"category"`
	Summary  string    `json:"summary"`
	Angle    string    `json:"angle"`
	Reason   string    `json:"reason"`
	Regions  []string  `json:"regions"`
	Articles []Article `json:"articles"`
}
type feed struct{ url, name, category string }

var feeds = []feed{
	{"https://www.antaranews.com/rss/politik.xml", "ANTARA", "Politik Indonesia"},
	{"https://www.antaranews.com/rss/terkini.xml", "ANTARA", "Indonesia"},
	{"https://www.antaranews.com/rss/tekno.xml", "ANTARA", "Tech"},
	{"https://techcrunch.com/feed/", "TechCrunch", "Tech"},
	{"https://feeds.arstechnica.com/arstechnica/index", "Ars Technica", "Tech"},
	{"https://www.bleepingcomputer.com/feed/", "BleepingComputer", "Siber"},
	{"https://krebsonsecurity.com/feed/", "KrebsOnSecurity", "Siber"},
	{"https://feeds.bbci.co.uk/news/world/rss.xml", "BBC", "Dunia"},
	{"https://www.aljazeera.com/xml/rss/all.xml", "Al Jazeera", "Dunia"},
	{"https://trends.google.com/trending/rss?geo=ID", "ID", "trend"},
	{"https://trends.google.com/trending/rss?geo=US", "US", "trend"},
	{"https://trends.google.com/trending/rss?geo=GB", "GB", "trend"},
}

type rss struct {
	Items []struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		Date        string `xml:"pubDate"`
	} `xml:"channel>item"`
}

var tags = regexp.MustCompile(`<[^>]*>`)

func clean(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tags.ReplaceAllString(s, " "))), " ")
}
func words(s string) map[string]bool {
	result := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(w) >= 3 && !strings.Contains("|the|and|for|with|from|yang|dan|dari|untuk|dalam|ini|itu|", "|"+w+"|") {
			result[w] = true
		}
	}
	return result
}
func matches(a, b string, min int) bool {
	x, y := words(a), words(b)
	n := 0
	for w := range x {
		if y[w] {
			n++
		}
	}
	shorter := minOf(len(x), len(y))
	return shorter > 0 && n >= min && n*3 >= shorter*2
}
func minOf(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func fetchFeed(f feed) (rss, error) {
	var data rss
	req, err := http.NewRequest(http.MethodGet, f.url, nil)
	if err != nil {
		return data, err
	}
	req.Header.Set("User-Agent", "BontokSadboy/1.0 (RSS reader)")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")
	reader := feedHTTP
	if req.URL.Hostname() == "www.antaranews.com" {
		reader = antaraFeedHTTP
	}
	res, err := reader.Do(req)
	if err != nil {
		return data, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return data, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	decoder := xml.NewDecoder(io.LimitReader(res.Body, 4<<20))
	decoder.Entity = xml.HTMLEntity
	err = decoder.Decode(&data)
	return data, err
}

var feedHTTP = &http.Client{Timeout: 12 * time.Second}

var antaraFeedHTTP = func() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	return &http.Client{Transport: transport, Timeout: feedHTTP.Timeout}
}()

type feedSnapshot struct {
	Topics        []Topic   `json:"topics"`
	UpdatedAt     time.Time `json:"updatedAt"`
	FailedSources int       `json:"failedSources"`
	Stale         bool      `json:"stale"`
}

var news struct {
	sync.Mutex
	data feedSnapshot
}

func refreshNews(force bool) feedSnapshot {
	news.Lock()
	defer news.Unlock()
	age := time.Since(news.data.UpdatedAt)
	if age < time.Minute || (!force && age < 30*time.Minute) {
		return news.data
	}
	type result struct {
		f    feed
		data rss
		err  error
	}
	results := make(chan result, len(feeds))
	for _, f := range feeds {
		go func() { d, e := fetchFeed(f); results <- result{f, d, e} }()
	}
	var batches []result
	failures := 0
	for range feeds {
		r := <-results
		if r.err != nil {
			failures++
			log.Printf("RSS %s (%s): %v", r.f.name, r.f.category, r.err)
		} else {
			batches = append(batches, r)
		}
	}
	snapshot := feedSnapshot{Topics: []Topic{}, UpdatedAt: time.Now(), FailedSources: failures}
	sort.SliceStable(batches, func(i, j int) bool {
		return batches[i].f.category == "Politik Indonesia" && batches[j].f.category != "Politik Indonesia"
	})
	for _, b := range batches {
		if b.f.category == "trend" {
			continue
		}
		for _, item := range b.data.Items {
			published, e := http.ParseTime(item.Date)
			if e != nil {
				published, e = time.Parse(time.RFC1123Z, item.Date)
			}
			maxAge := 72 * time.Hour
			if b.f.category == "Politik Indonesia" {
				maxAge = 7 * 24 * time.Hour
			}
			if e != nil || time.Since(published) > maxAge || time.Until(published) > 15*time.Minute {
				continue
			}
			link, e := url.Parse(strings.TrimSpace(item.Link))
			if e != nil || (link.Scheme != "https" && link.Scheme != "http") || link.Host == "" {
				continue
			}
			link.Fragment = ""
			title := clean(item.Title)
			if title == "" {
				continue
			}
			a := Article{title, b.f.name, link.String(), published}
			var matched bool
			for i := range snapshot.Topics {
				t := &snapshot.Topics[i]
				if t.URL == a.URL || matches(t.Title, title, 4) {
					if t.Source != a.Source {
						duplicate := false
						for _, old := range t.Articles {
							if old.Source == a.Source {
								duplicate = true
							}
						}
						if !duplicate {
							t.Articles = append(t.Articles, a)
						}
					}
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			h := fnv.New32a()
			_, _ = h.Write([]byte(a.URL))
			snapshot.Topics = append(snapshot.Topics, Topic{ID: h.Sum32(), Article: a, Category: b.f.category, Summary: clean(item.Description), Angle: "Apa yang terjadi dan kenapa ini penting?", Articles: []Article{a}})
		}
	}
	filtered := snapshot.Topics[:0]
	for _, t := range snapshot.Topics {
		for _, b := range batches {
			if b.f.category != "trend" {
				continue
			}
			for _, item := range b.data.Items {
				if matches(t.Title, clean(item.Title), 2) {
					t.Regions = append(t.Regions, b.f.name)
					break
				}
			}
		}
		// International general news only appears with evidence across regions and publishers.
		if t.Category == "Dunia" && (len(t.Regions) < 2 || len(t.Articles) < 2) {
			continue
		}
		t.Reason = "Berita terkini"
		if time.Since(t.Published) > 72*time.Hour {
			t.Reason = "Dalam sepekan"
		}
		if len(t.Articles) > 1 {
			t.Reason = "Diliput beberapa media"
		}
		if len(t.Regions) > 0 {
			t.Reason = "Pencarian naik di " + strings.Join(t.Regions, ", ")
		}
		filtered = append(filtered, t)
	}
	snapshot.Topics = filtered
	sort.SliceStable(snapshot.Topics, func(i, j int) bool {
		score := func(t Topic) float64 {
			v := float64(len(t.Regions)*30+len(t.Articles)*8) - time.Since(t.Published).Hours()
			if t.Category == "Politik Indonesia" {
				v += 10
			}
			return v
		}
		return score(snapshot.Topics[i]) > score(snapshot.Topics[j])
	})
	counts := map[string]int{}
	selected := snapshot.Topics[:0]
	for _, t := range snapshot.Topics {
		if counts[t.Category] >= 24 {
			continue
		}
		counts[t.Category]++
		selected = append(selected, t)
	}
	snapshot.Topics = selected
	if len(snapshot.Topics) == 0 && len(news.data.Topics) > 0 {
		previous := news.data
		previous.Stale = true
		previous.FailedSources = failures
		return previous
	}
	news.data = snapshot
	return snapshot
}
