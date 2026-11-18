import { useEffect, useState } from 'react'
import { ArrowUpRight, ArrowRight, Sparkles, Compass, Clapperboard, Files, Send, Cpu, Settings2, Radio, Globe, RefreshCw, Plus, Check, Play, Zap, ExternalLink, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Empty, EmptyHeader, EmptyTitle, EmptyDescription } from '@/components/ui/empty'
import { SidebarProvider, Sidebar, SidebarHeader, SidebarContent, SidebarGroup, SidebarGroupLabel, SidebarMenu, SidebarMenuItem, SidebarMenuButton, SidebarFooter, SidebarInset, SidebarTrigger } from '@/components/ui/sidebar'
import { Toaster } from '@/components/ui/sonner'
import { toast } from 'sonner'
import { api, SocialAccounts, PublishVideo } from './Social'
import { NavigationSync } from './NavigationSync'
import { renderAnimation } from './renderAnimation'
import { Settings, type Preferences } from './Settings'
import { picture, sampleScript, type Topic, type Draft } from './data'

const navigation = [{ id: 'today', label: 'Hari ini', icon: Sparkles }, { id: 'news', label: 'Jelajah berita', icon: Compass }, { id: 'studio', label: 'Studio video', icon: Clapperboard }, { id: 'drafts', label: 'Draft & video', icon: Files }, { id: 'publish', label: 'Publikasi', icon: Send }]
const settings = [{ id: 'model', label: 'Model AI', icon: Cpu }, { id: 'higgsfield', label: 'Higgsfield', icon: Zap }, { id: 'accounts', label: 'Akun sosial', icon: Radio }, { id: 'news-settings', label: 'Sumber & preferensi', icon: Settings2 }]
function read<T>(key: string, fallback: T): T {
  try { const value = localStorage.getItem(key); return value ? JSON.parse(value) : fallback } catch { return fallback }
}
function useStored<T>(key: string, fallback: T) {
  const [value, setValue] = useState<T>(() => read(key, fallback))
  useEffect(() => { try { localStorage.setItem(key, JSON.stringify(value)) } catch { toast.error('Penyimpanan lokal tidak tersedia') } }, [key, value])
  return [value, setValue] as const
}

export default function App() {
  const [page, setPage] = useState(new URLSearchParams(location.search).has('accounts') ? 'accounts' : 'today')
  const [drafts, setDrafts] = useStored<Draft[]>('bontok-drafts-v1', [])
  const [prefs, setPrefs] = useStored<Preferences>('bontok-prefs-v1', { provider: 'Codex / OpenAI', model: 'Default', political: true, international: true })
  const [activeId, setActiveId] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('Semua')
  const [detail, setDetail] = useState<Topic | null>(null)
  const [feed, setFeed] = useState<{topics: Topic[]; updatedAt: string; stale: boolean; failedSources: number} | null>(null)
  const [loading, setLoading] = useState(false)
  const [feedError, setFeedError] = useState('')
  const [media, setMedia] = useState<{ draftId: string; file: File; url: string } | null>(null)
  const [playable, setPlayable] = useState(false)
  const [aiBusy, setAiBusy] = useState(false)
  const [renderProgress, setRenderProgress] = useState('')
  const [renderError, setRenderError] = useState('')
  const [manualTopicOpen, setManualTopicOpen] = useState(false)
  async function loadNews(refresh = false) {
    setLoading(true); setFeedError('')
    try { setFeed(await api(refresh ? '/api/news/refresh' : '/api/news', refresh ? { method: 'POST' } : undefined)) }
    catch (e) { setFeedError(e instanceof Error ? e.message : 'Gagal memuat berita') }
    finally { setLoading(false) }
  }
  useEffect(() => { void loadNews(); const timer = setInterval(loadNews, 300_000); return () => clearInterval(timer) }, [])
  useEffect(() => () => { if (media) URL.revokeObjectURL(media.url) }, [media])
  const topics = (feed?.topics ?? []).filter(t => prefs.international || t.category !== 'Dunia')
  if (prefs.political) topics.sort((a, b) => Number(b.category === 'Politik Indonesia') - Number(a.category === 'Politik Indonesia'))
  const active = drafts.find(d => d.id === activeId) ?? drafts[0]
  const currentMedia = media?.draftId === active?.id ? media : null
  const filtered = topics.filter(t => (category === 'Semua' || category === t.category) && `${t.title} ${t.summary}`.toLowerCase().includes(query.toLowerCase()))
  const title = [...navigation, ...settings].find(n => n.id === page)?.label
  const updateDraft = (patch: Partial<Draft>, id = active?.id) => setDrafts(current => current.map(d => d.id === id ? { ...d, ...patch } : d))
  async function generateScript() {
    if (!active) return
    const topic = active.topic ?? topics.find(item => item.id === active.topicId)
    if (!topic) throw new Error('Topik berita draft ini tidak tersedia lagi')
    setAiBusy(true)
    try {
      const result = await api<Pick<Draft, 'title' | 'script' | 'caption' | 'scenes'>>('/api/ai/draft', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: topic.title, summary: topic.summary, angle: topic.angle, category: topic.category, model: prefs.model }),
      })
      updateDraft({ ...result, status: 'Draft', uploads: {} }, active.id)
      toast.success('Naskah dibuat oleh Codex')
      return result.scenes
    } finally { setAiBusy(false) }
  }
  function createDraft(t: Topic) {
    const draft: Draft = { id: crypto.randomUUID(), topicId: t.id, topic: t, title: t.title, script: sampleScript(t), caption: `${t.title}\nHati boleh kosong, kepala jangan. #BontokSadboy`, status: 'Draft', uploads: {} }
    setDrafts(current => [draft, ...current]); setActiveId(draft.id); setPage('studio'); setDetail(null); setRenderError('')
  }

  function createManualDraft(data: FormData) {
    const title = String(data.get('title') ?? '').trim()
    const summary = String(data.get('summary') ?? '').trim()
    const angle = String(data.get('angle') ?? '').trim()
    if (!title || !summary) { toast.error('Isi judul dan ringkasan fakta.'); return }
    createDraft({
      id: Date.now(), title, summary, angle, source: 'Topik manual', url: '',
      published: new Date().toISOString(), category: 'Topik manual', reason: '',
      articles: [],
    })
    setManualTopicOpen(false)
  }
  async function generateVideo() {
    if (!active || renderProgress) return
    setRenderError('')
    setRenderProgress('Menyiapkan video animasi…')
    try {
      await api('/api/render/ready')
      const scenes = active.scenes?.length ? active.scenes : await generateScript()
      if (!scenes?.length) throw new Error('Codex belum mengembalikan storyboard')
      const file = await renderAnimation(active.id, scenes, setRenderProgress)
      setPlayable(false)
      setMedia({ draftId: active.id, file, url: URL.createObjectURL(file) })
      updateDraft({ status: 'Menunggu review', uploads: {} }, active.id)
    } catch (e) {
      setRenderError(e instanceof Error ? e.message : 'Pembuatan video gagal')
    } finally {
      setRenderProgress('')
    }
  }
  const empty = <Empty><EmptyHeader><EmptyTitle>Mulai dari satu cerita.</EmptyTitle><EmptyDescription>Pilih topik yang menarik, lalu buat video pertamamu.</EmptyDescription></EmptyHeader><Button onClick={() => setPage('news')}>Jelajah berita <ArrowRight data-icon="inline-end" /></Button></Empty>
  return <SidebarProvider><NavigationSync page={page} /><Sidebar><SidebarHeader><a className="brand" href="#" onClick={e => { e.preventDefault(); setPage('today') }}><span className="brand-mark">b.</span><span>bontok<span className="brand-sub">SADBOY STUDIO</span></span></a></SidebarHeader><SidebarContent>
  <SidebarGroup><SidebarGroupLabel>WORKSPACE</SidebarGroupLabel><SidebarMenu>{navigation.map(item => <SidebarMenuItem key={item.id}><SidebarMenuButton isActive={page === item.id} onClick={() => setPage(item.id)}><item.icon /><span>{item.label}</span>{item.id === 'drafts' && drafts.length > 0 && <span className="nav-count">{drafts.length}</span>}</SidebarMenuButton></SidebarMenuItem>)}</SidebarMenu></SidebarGroup>
  <SidebarGroup><SidebarGroupLabel>PENGATURAN</SidebarGroupLabel><SidebarMenu>{settings.map(item => <SidebarMenuItem key={item.id}><SidebarMenuButton isActive={page === item.id} onClick={() => setPage(item.id)}><item.icon /><span>{item.label}</span></SidebarMenuButton></SidebarMenuItem>)}</SidebarMenu></SidebarGroup>
  </SidebarContent><SidebarFooter><div className="sidebar-note"><Sparkles /><p>Hati boleh kosong.<br /><strong>Kepala jangan.</strong></p></div><div className="profile"><span className="profile-icon">BS</span><span>Bontok Sadboy<small>Personal workspace</small></span></div></SidebarFooter></Sidebar>
  <SidebarInset><header className="topbar"><div><SidebarTrigger /><span className="breadcrumb">Workspace <span>/</span> <strong>{title}</strong></span></div><div><span className="topbar-date">{new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Asia/Jakarta' }).format(new Date())}</span></div></header><main>
  {(page === 'today' || page === 'news') && <><div className="page-heading"><div><span className="eyebrow">YOUR NEXT STORY STARTS HERE</span><h1>{page === 'today' ? 'Dunia punya cerita.' : 'Temukan cerita berikutnya.'}<span className="heading-muted"> Kamu yang bawakan.</span></h1><p>Pilih yang menarik. Beri sudut pandangmu. Jadikan video.</p></div><Button variant="outline" disabled={loading} onClick={() => loadNews(true)}><RefreshCw data-icon="inline-start" />{loading ? 'Memuat berita…' : 'Perbarui topik'}</Button></div>{feedError && <p role="alert" className="mb-4">{feedError}</p>}{feed?.failedSources ? <p role="status" className="mb-4 muted-copy">{feed.failedSources} sumber belum merespons. {feed.stale ? 'Menampilkan hasil terakhir.' : 'Sumber lain tetap ditampilkan.'}</p> : null}
  {page === 'today' && topics[0] && <><div className="metrics"><div><span>Topik pilihan</span><strong>{topics.length.toString().padStart(2, '0')}<small>untuk inspirasi hari ini</small></strong></div><div><span>Dalam pengerjaan</span><strong>{drafts.filter(d => d.status !== 'Siap tayang').length.toString().padStart(2, '0')}<small>cerita sedang diracik</small></strong></div><div><span>Siap tayang</span><strong>{drafts.filter(d => d.status === 'Siap tayang').length.toString().padStart(2, '0')}<small>tinggal bagikan</small></strong></div></div>
  <div className="feature-grid"><article className="hero-story" style={{ backgroundImage: `linear-gradient(90deg,rgba(0,0,0,.85),rgba(0,0,0,.1)),url(${picture(topics[0].image)})` }}><div className="hero-content"><Badge>TOPIK PILIHAN</Badge><span className="hero-category">{topics[0].category}</span><h2>{topics[0].title}</h2><p>{topics[0].angle}</p><div className="hero-actions"><Button onClick={() => createDraft(topics[0])}><Sparkles data-icon="inline-start" />Jadikan video</Button><Button variant="secondary" size="icon" aria-label="Detail topik utama" onClick={() => setDetail(topics[0])}><ArrowUpRight /></Button></div></div></article><aside className="daily-note"><span className="eyebrow">IDE UNTUK HARI INI</span><h2>Satu menit.<br />Sudut pandang<br /><em>yang berbeda.</em></h2><p>Mulai dari pertanyaan yang bikin penasaran. Biar penonton pulang membawa sesuatu.</p><Button variant="outline" onClick={() => setPage('studio')}>Buka studio <ArrowUpRight data-icon="inline-end" /></Button><div className="note-bottom"><span>9:16 VERTICAL</span><span>60 DETIK</span></div></aside></div></>}
  <div className="section-heading"><h2>{page === 'today' ? 'Layak kamu ceritakan' : 'Jelajah topik'}</h2><span>{feed ? new Intl.DateTimeFormat('id-ID', { timeStyle: 'short', timeZone: 'Asia/Jakarta' }).format(new Date(feed.updatedAt)) + ' WIB' : ''}</span></div><div className="news-toolbar"><ToggleGroup type="single" value={category} onValueChange={value => value && setCategory(value)} aria-label="Kategori berita">{['Semua', 'Tech', 'Siber', 'Politik Indonesia', 'Indonesia', 'Dunia'].map(c => <ToggleGroupItem key={c} value={c}>{c}</ToggleGroupItem>)}</ToggleGroup><Input aria-label="Cari berita" placeholder="Cari topik atau kata kunci…" value={query} onChange={e => setQuery(e.target.value)} /></div>
  {filtered.length ? <div className="news-grid">{filtered.map(t => <Card key={t.id} className="news-card"><button className="story-image" aria-label={`Baca ${t.title}`} onClick={() => setDetail(t)}><img src={picture(t.image)} alt="" loading="lazy" /><span className="story-image-overlay"><ArrowUpRight /></span></button><CardHeader><div className="story-meta"><Badge variant="secondary">{t.category}</Badge><span>{t.reason}</span></div><CardTitle>{t.title}</CardTitle><CardDescription>{t.angle}</CardDescription></CardHeader><CardContent><span className="source"><Globe />{t.source}</span></CardContent><CardFooter><Button variant="ghost" onClick={() => setDetail(t)}>Lihat cerita <ArrowUpRight data-icon="inline-end" /></Button><Button size="icon" variant="outline" aria-label={`Buat video ${t.title}`} onClick={() => createDraft(t)}><Plus /></Button></CardFooter></Card>)}</div> : <Empty><EmptyHeader><EmptyTitle>Tidak ada topik yang cocok</EmptyTitle><EmptyDescription>Coba kata kunci lain atau pilih kategori Semua.</EmptyDescription></EmptyHeader></Empty>}
  </>}
  {page === 'drafts' && <><div className="page-heading"><div><span className="eyebrow">KOLEKSI KREATIFMU</span><h1>Draft & video</h1><p>Semua cerita yang sedang kamu bangun.</p></div><Button onClick={() => setPage('news')}><Plus data-icon="inline-start" />Buat baru</Button></div>{drafts.length ? <div className="draft-list">{drafts.map(d => <Card key={d.id}><CardHeader><CardTitle>{d.title}</CardTitle><CardDescription>{d.status} · 60 detik · 9:16</CardDescription></CardHeader><CardFooter><Button variant="outline" onClick={() => { setActiveId(d.id); setPage('studio') }}>Buka studio</Button>{d.status === 'Siap tayang' && <Button onClick={() => { setActiveId(d.id); setPage('publish') }}>Publikasikan</Button>}</CardFooter></Card>)}</div> : empty}</>}
  {page === 'studio' && <><div className="page-heading"><div><span className="eyebrow">DARI IDE JADI CERITA</span><h1>Studio video</h1><p>Racik naskahmu, lalu lihat bagaimana ceritanya berjalan.</p></div><div className="flex flex-wrap items-center gap-2">{active && <Badge variant="secondary">{active.status}</Badge>}<Button variant="outline" disabled={aiBusy || !!renderProgress} onClick={() => setManualTopicOpen(true)}><Plus data-icon="inline-start" />Topik manual</Button></div></div>{active ? <div className="studio-grid"><Card><CardHeader><CardTitle>Naskah & cerita</CardTitle><CardDescription>Perubahan tersimpan otomatis di browser ini.</CardDescription></CardHeader><CardContent><FieldGroup><Field><FieldLabel htmlFor="draft-title">Judul video</FieldLabel><Input id="draft-title" value={active.title} onChange={e => updateDraft({ title: e.target.value, status: 'Draft', uploads: {} })} /></Field><Field><div className="flex flex-wrap items-center justify-between gap-3"><FieldLabel htmlFor="script">Naskah</FieldLabel><Button type="button" variant="outline" size="sm" disabled={aiBusy || !!renderProgress} onClick={() => { void generateScript().catch(e => toast.error(e instanceof Error ? e.message : "Naskah gagal dibuat")) }}>{aiBusy ? <><Loader2 className="animate-spin" />Menyusun naskah…</> : <><Sparkles />Buat naskah dengan Codex</>}</Button></div><Textarea id="script" className="script-input" value={active.script} onChange={e => updateDraft({ script: e.target.value, status: 'Draft', uploads: {} })} /></Field></FieldGroup><div className="script-meta"><span>{active.script.trim().split(/\s+/).length} kata</span><span>Target 60 detik</span></div></CardContent><CardFooter><div className="space-y-2"><Button onClick={generateVideo} disabled={aiBusy || !!renderProgress}><Sparkles data-icon="inline-start" />{renderProgress ? "Sedang membuat video…" : renderError ? "Lanjutkan render" : "Buat video animasi"}</Button>{renderProgress && <p role="status">{renderProgress}</p>}{renderError && <p role="alert">{renderError}</p>}</div>
    {currentMedia && playable && active.status !== 'Siap tayang' && <Button variant="outline" disabled={!active.title.trim()} onClick={() => updateDraft({ status: 'Siap tayang' })}><Check data-icon="inline-start" />Setujui video</Button>}
    {currentMedia && playable && active.status === 'Siap tayang' && <Button onClick={() => setPage('publish')}>Lanjut publikasi <ArrowRight data-icon="inline-end" /></Button>}</CardFooter></Card>
    <div><div className="section-heading"><h2>Pratinjau video</h2></div>{currentMedia ? <video key={currentMedia.url} className="video-player" src={currentMedia.url} controls playsInline onLoadedMetadata={() => setPlayable(true)} onError={() => { setPlayable(false); toast.error('Video tidak bisa diputar di browser ini') }} /> : <div className="preview-placeholder"><Clapperboard /><p>Hasil animasi akan tampil di sini setelah render selesai.</p></div>}</div></div> : empty}</>}
  {page === 'publish' && <><div className="page-heading"><div><h1>Publikasi</h1><p>Atur caption dan pilih akun tujuan.</p></div></div>{active?.status === 'Siap tayang' && currentMedia && playable ? <PublishVideo key={active.id} draft={active} file={currentMedia.file} preview={currentMedia.url} save={updateDraft} connect={() => setPage('accounts')} /> : <Empty><EmptyHeader><EmptyTitle>Buat dan setujui video dahulu</EmptyTitle><EmptyDescription>Buat video animasi di Studio, tinjau hasilnya, lalu setujui untuk publikasi.</EmptyDescription></EmptyHeader><Button onClick={() => setPage('studio')}>Buka studio</Button></Empty>}</>}
  {page === 'studio' && !!active?.scenes?.length && (
    <Card className="mt-6">
      <CardHeader>
        <CardTitle>Storyboard animasi</CardTitle>
        <CardDescription>Lihat urutan adegan, narasi, dan prompt RCTFC.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {active.scenes.map((scene, index) => (
          <details key={index} className="rounded-md border p-4">
            <summary className="cursor-pointer">Adegan {index + 1} · {scene.seconds} detik · {scene.subtitle}</summary>
            <p className="mt-3">{scene.narration}</p>
            <pre className="my-3 whitespace-pre-wrap break-words text-sm">{scene.prompt}</pre>
            <Button variant="outline" onClick={async () => {
              try { await navigator.clipboard.writeText(scene.prompt); toast.success('Prompt RCTFC disalin') }
              catch { toast.error('Salin prompt langsung dari teks adegan') }
            }}>Salin prompt RCTFC</Button>
          </details>
        ))}
      </CardContent>
    </Card>
  )}
  {page === 'accounts' && <SocialAccounts />}
  {page !== 'accounts' && settings.some(s => s.id === page) && <Settings key={page} section={page === 'news-settings' ? 'news' : page} prefs={prefs} update={setPrefs} />}
  <footer className="page-footer"><span>BONTOK SADBOY STUDIO</span><span>Made for the curious.</span></footer>
  </main></SidebarInset>
  <Dialog open={!!detail} onOpenChange={open => !open && setDetail(null)}><DialogContent><DialogHeader><DialogTitle>{detail?.title}</DialogTitle><DialogDescription>{detail?.category} · {detail?.reason}</DialogDescription></DialogHeader><p>{detail?.summary}</p><blockquote>{detail?.angle}</blockquote>{detail?.articles?.map(article => <a key={article.url} className="source-link" href={article.url} target="_blank" rel="noreferrer">{article.source} · {new Date(article.published).toLocaleDateString("id-ID", { timeZone: 'Asia/Jakarta' })}<ExternalLink /></a>)}<DialogFooter><Button onClick={() => detail && createDraft(detail)}><Sparkles data-icon="inline-start" />Buat video dari topik ini</Button></DialogFooter></DialogContent></Dialog>
  <Dialog open={manualTopicOpen} onOpenChange={setManualTopicOpen}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Topik manual</DialogTitle>
        <DialogDescription>Masukkan fakta dan sudut cerita yang ingin dijadikan animasi.</DialogDescription>
      </DialogHeader>
      <form onSubmit={e => { e.preventDefault(); createManualDraft(new FormData(e.currentTarget)) }}>
        <FieldGroup>
          <Field><FieldLabel htmlFor="manual-title">Judul topik</FieldLabel><Input id="manual-title" name="title" placeholder="Contoh: Kasus Bridgeplay dan NotProton" required maxLength={150} /></Field>
          <Field><FieldLabel htmlFor="manual-summary">Ringkasan fakta</FieldLabel><Textarea id="manual-summary" name="summary" placeholder="Apa yang terjadi? Tulis fakta yang sudah kamu cek." required maxLength={1500} /></Field>
          <Field><FieldLabel htmlFor="manual-angle">Sudut cerita (opsional)</FieldLabel><Input id="manual-angle" name="angle" placeholder="Contoh: Apa itu fork dan kenapa atribusi penting?" maxLength={500} /></Field>
        </FieldGroup>
        <DialogFooter className="mt-4"><Button type="submit">Buat draft</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
  <Toaster theme="dark" /></SidebarProvider>
}

