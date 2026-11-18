import { useEffect } from 'react'
import { useSidebar } from '@/components/ui/sidebar'
export function NavigationSync({ page }: { page: string }) {
  const { setOpenMobile } = useSidebar()
  useEffect(() => { setOpenMobile(false) }, [page, setOpenMobile])
  return null
}
