export type Article = { title: string; source: string; url: string; published: string }
export type Topic = Article & { id: number; category: string; image?: string; summary: string; angle: string; reason: string; articles: Article[] }
export const picture = (id?: string) => `https://images.unsplash.com/${id ?? 'photo-1495020689067-958852a7765e'}?auto=format&fit=crop&w=1000&q=80`
export type AnimationScene = { seconds: number; narration: string; subtitle: string; visual: string; prompt: string }
export type Draft = { id: string; topicId: number; topic?: Topic; title: string; script: string; status: 'Draft' | 'Menunggu review' | 'Siap tayang'; caption: string; scenes?: AnimationScene[]; uploads: Record<string, string>; publications?: Record<string, { id?: string; status: string; error?: string }> }
export const platforms = ['TikTok', 'YouTube', 'Instagram']
export const sampleScript = (topic: Topic) => `${topic.title}\n\n${topic.summary}\n\nSumber: ${topic.url}`
