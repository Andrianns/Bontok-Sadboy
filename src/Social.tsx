import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent, CardFooter, CardDescription } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { platforms, type Draft } from './data'

type Account = { configured: boolean; connected: boolean; callback: string }
type Accounts = Record<string, Account>
type Result = { id?: string; status: string; error?: string }
type Creator = { creator_nickname: string; privacy_level_options: string[]; max_video_post_duration_sec: number; comment_disabled: boolean; duet_disabled: boolean; stitch_disabled: boolean }
export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, options)
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.error || `Layanan tidak tersedia (HTTP ${response.status})`)
  }
  if (response.status === 204) return undefined as T
  return response.json()
}
const message = (e: unknown) => e instanceof Error ? e.message : 'Terjadi kesalahan'
export function useAccounts() {
  const [accounts, setAccounts] = useState<Accounts>({})
  const [error, setError] = useState('')
  const reload = async () => {
    try { setAccounts(await api<Accounts>('/api/accounts')); setError('') }
    catch (e) { setError(message(e)) }
  }
  useEffect(() => { void reload() }, [])
  return { accounts, error, reload }
}
export function SocialAccounts() {
  const { accounts, error, reload } = useAccounts()
  const [actionError, setActionError] = useState('')
  const callbackResult = new URLSearchParams(window.location.search).get('accounts')
  return <div className="settings-page"><div className="page-heading"><div><h1>Akun sosial</h1><p>Hubungkan akun melalui halaman otorisasi platform.</p></div></div>
    {(error || actionError || callbackResult === 'error') && <p role="alert">{error || actionError || 'Otorisasi tidak selesai. Periksa kredensial aplikasi dan callback.'}</p>}
    {platforms.map(name => {
      const key = name.toLowerCase()
      const account = accounts[key]
      return <Card key={key}><CardHeader><CardTitle>{name}</CardTitle><CardDescription>{account?.connected ? 'Akun terhubung' : 'Belum terhubung'}</CardDescription></CardHeader>
        <CardContent>{account && !account.configured && <p className="muted-copy">Konfigurasikan aplikasi developer dan daftarkan callback <code className="break-all">{account.callback}</code>.</p>}</CardContent>
        <CardFooter>{account?.connected ? <Button variant="outline" onClick={async () => {
          try { setActionError(''); await api(`/api/accounts/${key}`, { method: 'DELETE' }); await reload() }
          catch (e) { setActionError(message(e)) }
        }}>Putuskan koneksi</Button> : account?.configured ? <Button asChild><a href={`/api/oauth/${key}/start`}>Hubungkan {name}</a></Button> : <Button disabled>Konfigurasi diperlukan</Button>}</CardFooter>
      </Card>
    })}
  </div>
}
export function PublishVideo({ draft, file, preview, save, connect }: { draft: Draft; file: File; preview: string; save: (patch: Partial<Draft>) => void; connect: () => void }) {
  const { accounts, error } = useAccounts()
  const [destinations, setDestinations] = useState<string[]>([])
  const [results, setResults] = useState<Record<string, Result>>(draft.publications ?? {})
  const [busy, setBusy] = useState(false)
  const [creator, setCreator] = useState<Creator | null>(null)
  const [privacy, setPrivacy] = useState('')
  const [youtubePrivacy, setYoutubePrivacy] = useState('private')
  const [errorText, setErrorText] = useState('')
  const [comments, setComments] = useState(false)
  const [duet, setDuet] = useState(false)
  const [stitch, setStitch] = useState(false)
  const [duration, setDuration] = useState(0)
  const ready = (p: string) => accounts[p]?.connected && !results[p]?.id && results[p]?.status !== 'Mengirim'
  async function select(keys: string[]) {
    setErrorText(''); setCreator(null); setPrivacy(''); setDestinations(keys)
    setComments(false); setDuet(false); setStitch(false)
    if (keys.includes('tiktok')) {
      try { setCreator(await api<Creator>('/api/tiktok/creator')) }
      catch (e) { setErrorText(message(e)) }
    }
  }
  function record(next: Record<string, Result>) { setResults(next); save({ publications: next }) }
  async function send() {
    const keys = destinations
    setBusy(true); setDestinations([])
    let next = { ...results }
    for (const p of keys) {
      next = { ...next, [p]: { status: 'Mengirim' } }; record(next)
      const data = new FormData()
      data.set('video', file); data.set('title', draft.title); data.set('caption', draft.caption)
      data.set('consent', 'yes'); data.set('privacy', p === 'youtube' ? youtubePrivacy : privacy)
      data.set('disable_comment', String(!comments)); data.set('disable_duet', String(!duet)); data.set('disable_stitch', String(!stitch))
      try { next = { ...next, [p]: await api<Result>(`/api/publish/${p}`, { method: 'POST', body: data }) } }
      catch (e) { next = { ...next, [p]: { status: 'Periksa akun', error: message(e) } } }
      record(next)
    }
    setBusy(false)
  }
  async function check(id: string) {
    try {
      const result = await api<Result>(`/api/tiktok/status?id=${encodeURIComponent(id)}`)
      record({ ...results, tiktok: { id: result.status === 'Gagal' ? undefined : id, status: result.status } })
    } catch (e) { setErrorText(message(e)) }
  }
  return <div className="studio-grid"><Card>
    <CardHeader><CardTitle>{draft.title}</CardTitle><CardDescription>Video siap dipublikasikan</CardDescription></CardHeader>
    <CardContent><FieldGroup><Field><FieldLabel htmlFor="caption">Caption</FieldLabel><Textarea id="caption" disabled={busy} value={draft.caption} onChange={e => save({ caption: e.target.value })} /></Field></FieldGroup>
    {(error || errorText) && <p role="alert">{error || errorText}</p>}
    <div className="platform-list">{platforms.map(name => {
      const p = name.toLowerCase(), result = results[p]
      return <div key={p}><span><strong>{name}</strong><small>{result?.error || (accounts[p]?.connected ? 'Terhubung' : 'Belum terhubung')}</small></span>
        <Badge variant="outline">{result?.status || 'Belum diunggah'}</Badge>
        {p === 'tiktok' && result?.id && result.status === 'Diproses' ? <Button variant="outline" disabled={busy} onClick={() => check(result.id!)}>Cek status</Button> : <Button variant="outline" disabled={busy || !ready(p)} onClick={() => select([p])}>Upload</Button>}
      </div>
    })}</div></CardContent>
    <CardFooter><Button variant="outline" onClick={connect}>Kelola akun</Button><Button disabled={busy || !platforms.some(p => ready(p.toLowerCase()))} onClick={() => select(platforms.map(p => p.toLowerCase()).filter(ready))}>{busy ? 'Mengunggah…' : 'Upload ke semua akun terhubung'}</Button></CardFooter>
    </Card><video className="video-player" src={preview} controls playsInline onLoadedMetadata={e => setDuration(e.currentTarget.duration)} />
    <Dialog open={destinations.length > 0} onOpenChange={open => !open && setDestinations([])}>
      <DialogContent><DialogHeader><DialogTitle>Konfirmasi publikasi</DialogTitle><DialogDescription>Video akan dikirim ke {platforms.filter(p => destinations.includes(p.toLowerCase())).join(', ')}.</DialogDescription></DialogHeader>
        {destinations.includes('youtube') && <label className="flex flex-col gap-2">Visibilitas YouTube<select className="rounded border bg-background p-2" value={youtubePrivacy} onChange={e => setYoutubePrivacy(e.target.value)}><option value="private">Privat</option><option value="unlisted">Tidak tercantum</option><option value="public">Publik</option></select></label>}
        {destinations.includes('tiktok') && (creator ? <div className="flex flex-col gap-2"><p>Akun TikTok {creator.creator_nickname}</p>
          <label className="flex flex-col gap-2">Visibilitas TikTok<select className="rounded border bg-background p-2" value={privacy} onChange={e => setPrivacy(e.target.value)}><option value="">Pilih visibilitas</option>{creator.privacy_level_options?.map(option => <option key={option}>{option}</option>)}</select></label>
          <label><input type="checkbox" checked={comments} disabled={creator.comment_disabled} onChange={e => setComments(e.target.checked)} /> Izinkan komentar</label>
          <label><input type="checkbox" checked={duet} disabled={creator.duet_disabled} onChange={e => setDuet(e.target.checked)} /> Izinkan Duet</label>
          <label><input type="checkbox" checked={stitch} disabled={creator.stitch_disabled} onChange={e => setStitch(e.target.checked)} /> Izinkan Stitch</label>
          {duration > creator.max_video_post_duration_sec && <p role="alert">Durasi video melebihi batas akun TikTok.</p>}
        </div> : <p role={errorText ? 'alert' : 'status'}>{errorText || 'Memeriksa akun TikTok…'}</p>)}
        <DialogFooter><Button variant="outline" onClick={() => setDestinations([])}>Batal</Button><Button disabled={busy || (destinations.includes('tiktok') && (!creator || !privacy || !duration || duration > creator.max_video_post_duration_sec))} onClick={send}>Setujui dan upload</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
}

