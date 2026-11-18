# Bontok Sadboy

Dashboard React dan backend Go untuk kurasi berita dan publikasi video.

## Jalankan lokal

Dari root repo, jalankan `npm ci` dan `npm run build`. Lalu jalankan:

```powershell
Set-Location backend
go run .
```

Buka http://127.0.0.1:8080/. Backend menyajikan hasil build React dari `../dist` dan mengambil feed RSS setiap 30 menit. Tombol Perbarui topik meminta penyegaran dengan jeda minimal satu menit. Untuk pengembangan UI, Vite meneruskan `/api` ke backend yang sama.

## Naskah AI dengan Codex

Pasang Codex CLI dan login pada komputer yang menjalankan backend (`codex login`). Di Pengaturan → Model AI, dashboard memeriksa sesi CLI dan dapat memulai login jika belum terhubung. Di Studio, pilih “Buat naskah dengan Codex” untuk menghasilkan judul, narasi sekitar 60 detik, dan caption dari topik berita. Sesi login tetap dikelola Codex; backend lokal menjalankan CLI dalam mode ephemeral dan read-only. Model dapat dikosongkan agar mengikuti default Codex atau diisi dengan ID model yang tersedia pada akun.

## Hubungkan akun sosial

Jalankan dashboard sebagai workspace pribadi. Untuk akses jarak jauh, letakkan reverse proxy HTTPS di depan backend loopback dan arahkan seluruh path, termasuk `/api` dan `/media`, ke backend. Batasi akses dashboard dengan `APP_USERNAME` dan `APP_PASSWORD`. Path `/media` memakai tautan acak sementara agar Instagram dapat mengambil video selama pemrosesan.

Set variabel lingkungan berikut pada proses Go. Simpan nilai rahasia di konfigurasi host, bukan di repo atau variabel frontend.

| Variabel | Nilai |
| --- | --- |
| `APP_ORIGIN` | Origin publik HTTPS, tanpa trailing slash. Untuk YouTube lokal dapat memakai origin loopback yang terdaftar di Google Cloud. |
| `APP_USERNAME`, `APP_PASSWORD` | Login workspace. Password wajib minimal 16 karakter untuk origin HTTPS. |
| `TOKEN_SECRET` | Rahasia acak minimal 32 karakter. Gunakan nilai tetap untuk membaca token terenkripsi setelah restart. |
| `STATE_DIR` | Direktori persisten untuk token. Default `.state` dalam folder backend. |
| `YOUTUBE_CLIENT_ID`, `YOUTUBE_CLIENT_SECRET` | OAuth client jenis Web application dengan YouTube Data API diaktifkan. |
| `TIKTOK_CLIENT_KEY`, `TIKTOK_CLIENT_SECRET` | Aplikasi TikTok Login Kit Web dan Content Posting API dengan scope `user.info.basic` dan `video.publish`. |
| `INSTAGRAM_CLIENT_ID`, `INSTAGRAM_CLIENT_SECRET` | Aplikasi Meta dengan Instagram API with Instagram Login dan izin publikasi untuk akun profesional. |
| `INSTAGRAM_GRAPH_VERSION` | Versi Graph API yang didukung aplikasi Meta, dalam format `vNN.N`. |

Daftarkan callback yang tampil di Pengaturan → Akun sosial pada dashboard developer. Polanya:

- `APP_ORIGIN/api/oauth/youtube/callback`
- `APP_ORIGIN/api/oauth/tiktok/callback`
- `APP_ORIGIN/api/oauth/instagram/callback`

Restart Go setelah konfigurasi berubah, buka Akun sosial, lalu klik Hubungkan. Token tidak dikirim ke browser dan tersimpan terenkripsi di server. TikTok Web dan Instagram memerlukan origin HTTPS yang dapat diakses platform. Aplikasi dan akun harus memenuhi izin dan peninjauan masing-masing platform. Google dan TikTok dapat membatasi visibilitas unggahan dari aplikasi yang belum diaudit.

## Publikasi

Pilih topik, buka Studio, lalu klik **Buat video animasi**. Dashboard merender sepuluh adegan di Higgsfield dan menggabungkannya dengan subtitle. Tinjau hasilnya, setujui, lalu pilih akun tujuan di Publikasi. TikTok memberi ID unggahan dan tombol Cek status selama pemrosesan. Jika koneksi terputus setelah mulai upload, cek akun tujuan sebelum mengulang agar video tidak terbit dua kali.

Render memakai plugin Higgsfield yang terhubung pada Codex lokal. Login Codex dan hubungkan akun Higgsfield di Codex sebelum menjalankan backend. Studio mengirim prompt RCTFC dan dialog Indonesia ke Codex, menunggu hasil plugin, lalu menggabungkan klip beserta suaranya. Status pekerjaan disimpan di folder state backend. Penggabungan video memakai FFmpeg yang dipasang oleh npm pada platform yang didukung atau FFmpeg dengan libass di PATH. Pilihan kredit atau unlimited yang membutuhkan keputusan pengguna ditampilkan sebagai pesan di Studio. Jika backend dimulai ulang saat render berlangsung, periksa riwayat Higgsfield untuk memastikan hasil sebelum memulai pekerjaan baru.

- [YouTube OAuth](https://developers.google.com/youtube/data/guides/authentication)
- [TikTok Login Kit Web](https://developers.tiktok.com/docs/en/login-kit-web)
- [TikTok Direct Post](https://developers.tiktok.com/docs/en/content-posting-api-get-started)
- [Instagram API](https://www.postman.com/meta/instagram/documentation/6yqw8pt/instagram-api)


TikTok menyatakan Direct Post ditujukan untuk aplikasi dengan audiens luas dan tidak menerima alat internal untuk akun milik sendiri atau tim. Lihat pedoman Content Sharing sebelum mengandalkan unggahan publik. https://developers.tiktok.com/docs/en/content-sharing-guidelines
