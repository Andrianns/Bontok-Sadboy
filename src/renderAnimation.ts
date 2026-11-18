import { api } from './Social'
import type { AnimationScene } from './data'

type Job = { request_id: string; status: string; message?: string }
const failed = ['failed', 'canceled', 'nsfw']

export async function renderAnimation(
  draftId: string, scenes: AnimationScene[], progress: (text: string) => void,
) {
  if (scenes.length !== 10) throw new Error('Buat storyboard berisi sepuluh adegan terlebih dahulu.')
  await api('/api/render/ready')
  const jobs: Job[] = []
  for (const [index, scene] of scenes.entries()) {
    const key = `bontok-plugin-render:${draftId}:${index}:${scene.prompt}:${scene.narration}`
    const saved = localStorage.getItem(key)
    let job = saved ? JSON.parse(saved) as Job : undefined
    if (job && !job.request_id) {
      throw new Error(`Pengiriman adegan ${index + 1} belum bisa dikonfirmasi. Periksa riwayat Higgsfield sebelum mengirim ulang.`)
    }
    if (!job || failed.includes(job.status)) {
      progress(`Mengirim adegan ${index + 1} dari ${scenes.length} ke Higgsfield…`)
      localStorage.setItem(key, JSON.stringify({ status: 'sending' }))
      try {
        job = await api<Job>('/api/render/clip', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ prompt: scene.prompt, narration: scene.narration }),
        })
      } catch (error) {
        if (error instanceof Error && /Higgsfield menolak permintaan|Prompt klip|Atur HF_API_KEY/.test(error.message)) {
          localStorage.removeItem(key)
        }
        throw error
      }
      localStorage.setItem(key, JSON.stringify(job))
    }
    const deadline = Date.now() + 26 * 60_000
    while (job.status !== 'completed') {
      if (['blocked', 'needs_action', ...failed].includes(job.status)) {
        throw new Error(job.message || `Adegan ${index + 1} memerlukan pemeriksaan di Higgsfield.`)
      }
      progress(`Plugin Higgsfield sedang merender adegan ${index + 1}/${scenes.length} · ${index} adegan selesai`)
      if (Date.now() > deadline) throw new Error('Render masih berjalan. Klik Lanjutkan render untuk memeriksa kembali.')
      await new Promise(resolve => setTimeout(resolve, 5000))
      const result: Job = await api<Job>('/api/render/clip/' + encodeURIComponent(job.request_id))
      job = { ...result, request_id: job.request_id }
      localStorage.setItem(key, JSON.stringify(job))
    }
    jobs.push(job)
  }

  progress('Menggabungkan klip, suara, dan subtitle…')
  const response = await fetch('/api/render/assemble', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(jobs.map((job, index) => ({ id: job.request_id, subtitle: scenes[index].subtitle }))),
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.error || 'Video akhir gagal digabungkan')
  }
  return new File([await response.blob()], 'bontok-sadboy.mp4', { type: 'video/mp4' })
}
