import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Field, FieldGroup, FieldLabel, FieldDescription } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { api } from './Social'
export type Preferences = { provider: string; model: string; political: boolean; international: boolean }
export function Settings({ section, prefs, update }: { section: string; prefs: Preferences; update: (p: Preferences) => void }) {
  const [codex, setCodex] = useState<{ loggedIn: boolean; message: string } | null>(null)
  const [loginBusy, setLoginBusy] = useState(false)
  const [statusError, setStatusError] = useState('')
  async function refreshCodex() {
    try { const status = await api<{ loggedIn: boolean; message: string }>('/api/ai/status'); setCodex(status); setStatusError('') }
    catch (e) { setStatusError(e instanceof Error ? e.message : 'Status Codex tidak tersedia') }
  }
  useEffect(() => {
    if (section !== 'model') return
    void refreshCodex()
    const timer = window.setInterval(refreshCodex, 5000)
    return () => window.clearInterval(timer)
  }, [section])
  const title = section === 'model' ? 'Model AI' : section === 'higgsfield' ? 'Higgsfield' : section === 'accounts' ? 'Akun sosial' : 'Sumber & preferensi'
  return <div className="settings-page"><div className="page-heading"><div><span className="eyebrow">PENGATURAN WORKSPACE</span><h1>{title}</h1><p>Sesuaikan studio dengan cara kamu bekerja.</p></div></div>
  {<Card><CardHeader><CardTitle>{title === 'Model AI' ? 'Provider & model' : title}</CardTitle><CardDescription>{section === 'news' ? 'Prioritas berita untuk koleksi harianmu.' : 'Pengaturan untuk alur produksi video.'}</CardDescription></CardHeader><CardContent><FieldGroup>
  {section === 'news' ? <><Field orientation="horizontal"><FieldLabel htmlFor="political">Utamakan politik Indonesia</FieldLabel><Switch id="political" checked={prefs.political} onCheckedChange={political => update({ ...prefs, political })} /></Field><Field orientation="horizontal"><FieldLabel htmlFor="international">Sertakan isu internasional besar</FieldLabel><Switch id="international" checked={prefs.international} onCheckedChange={international => update({ ...prefs, international })} /></Field><FieldDescription>Preferensi diterapkan pada tampilan feed.</FieldDescription></> : <>
  {section === 'model' && <><Field><FieldLabel>Akun Codex</FieldLabel><div className="flex flex-wrap items-center gap-3"><Badge variant={codex?.loggedIn ? 'default' : 'secondary'}>{codex?.loggedIn ? 'Terhubung' : 'Belum terhubung'}</Badge><span className="muted-copy">{codex?.message ?? 'Memeriksa status login…'}</span></div>{statusError && <p role="alert">{statusError}</p>}</Field><Field><FieldLabel htmlFor="model-name">Model Codex (opsional)</FieldLabel><Input id="model-name" placeholder="Default dari Codex" value={prefs.model === 'Default' ? '' : prefs.model} onChange={e => update({ ...prefs, provider: 'Codex', model: e.target.value || 'Default' })} /><FieldDescription>Kosongkan untuk memakai model default dari konfigurasi Codex.</FieldDescription></Field></>}
  <FieldDescription>{section === "higgsfield" ? "Studio memakai plugin Higgsfield yang terhubung ke Codex. Pastikan akun Higgsfield sudah terhubung pada Codex di komputer ini." : section === 'model' ? "Studio memakai Codex CLI yang sudah login pada komputer ini. Login dan kredensial tetap dikelola Codex." : "Preferensi berita diterapkan pada feed."}</FieldDescription>
  </>}
  </FieldGroup></CardContent><CardFooter>{section === "higgsfield" ? <Button asChild><a href="https://higgsfield.ai/" target="_blank" rel="noreferrer">Buka Higgsfield</a></Button> : section === 'model' ? <><Button disabled={loginBusy || codex?.loggedIn} onClick={async () => { setLoginBusy(true); try { const result = await api<{ message?: string }>('/api/ai/login', { method: 'POST' }); toast(result.message || 'Login Codex dimulai'); await refreshCodex() } catch (e) { toast.error(e instanceof Error ? e.message : 'Login Codex gagal') } finally { setLoginBusy(false) } }}>{loginBusy ? 'Membuka login…' : codex?.loggedIn ? 'Codex terhubung' : 'Hubungkan Codex'}</Button><Button variant="outline" onClick={() => { void refreshCodex(); toast('Status Codex diperbarui') }}>Periksa status</Button></> : <Button onClick={() => toast("Preferensi tersimpan")}>Simpan preferensi</Button>}</CardFooter></Card>}
  </div>
}
