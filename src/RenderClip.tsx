import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { api } from './Social'

type RenderJob = { request_id: string; status: string; video?: { url: string } }
const terminal = ['completed', 'failed', 'canceled', 'nsfw']

export function RenderClip({ prompt, storageKey }: { prompt: string; storageKey: string }) {
  const [job, setJob] = useState<RenderJob | null>(() => {
    try { return JSON.parse(localStorage.getItem(storageKey) ?? 'null') }
    catch { return null }
  })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!job) return
    try { localStorage.setItem(storageKey, JSON.stringify(job)) } catch { /* Preview remains available in this tab. */ }
    if (terminal.includes(job.status)) return
    const timer = window.setTimeout(async () => {
      try {
        const next = await api<RenderJob>('/api/render/clip/' + encodeURIComponent(job.request_id))
        setError('')
        setJob({ ...next, request_id: job.request_id })
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Status render tidak tersedia')
        setJob({ ...job })
      }
    }, error ? 15000 : 5000)
    return () => window.clearTimeout(timer)
  }, [job, storageKey, error])

  async function render() {
    setBusy(true)
    setError('')
    try {
      const next = await api<RenderJob>('/api/render/clip', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt }),
      })
      setJob(next)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Render gagal dimulai')
    } finally {
      setBusy(false)
    }
  }

  return <div className="mt-3 space-y-3">
    <Button type="button" disabled={busy || (!!job && !terminal.includes(job.status))} onClick={render}>
      {busy ? 'Mengirim ke Higgsfield…' : job && !terminal.includes(job.status) ? 'Sedang dirender…' : job ? 'Render ulang klip' : 'Render klip · 6 detik'}
    </Button>
    {job && <p role="status">Status Higgsfield: {job.status || 'queued'}</p>}
    {error && <p role="alert">{error}</p>}
    {job?.video?.url && <video className="max-h-96 rounded-md" src={job.video.url} controls playsInline />}
  </div>
}
