import { useEffect, useRef } from 'react'
import { useSession } from './useSession'

const ACTIVE_LINE = 0.33 // active word sits this fraction down from the top

export default function Overlay() {
  const { script, state, status } = useSession()
  const box = useRef<HTMLDivElement>(null)
  const cursor = state?.cursor ?? 0

  useEffect(() => {
    const el = box.current?.querySelector<HTMLElement>('.w.active')
    if (el && box.current) {
      box.current.scrollTo({ top: el.offsetTop - box.current.clientHeight * ACTIVE_LINE, behavior: 'smooth' })
    }
  }, [cursor, script])

  const parts: React.ReactNode[] = []
  if (script) {
    let pos = 0
    const words = script.words ?? []
    words.forEach((w, i) => {
      if (w.s > pos) parts.push(script.text.slice(pos, w.s))
      const cls = cursor > w.last ? 'read' : cursor >= w.first ? 'active' : ''
      const tent = cls === 'active' && (state?.tentativeUntil ?? 0) > 0 ? ' tentative' : ''
      parts.push(
        <span key={i} className={`w ${cls}${tent}`}>
          {script.text.slice(w.s, w.e)}
        </span>,
      )
      pos = w.e
    })
    parts.push(script.text.slice(pos))
  }

  return (
    <div className="overlay">
      <div className="drag" />
      {state?.mode === 'improv' && <div className="offscript">Off-Script</div>}
      <div className="script" ref={box}>
        {script ? parts : <span className="hint">Start a session from the editor ({status})</span>}
      </div>
    </div>
  )
}
