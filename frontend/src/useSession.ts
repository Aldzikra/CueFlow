import { useEffect, useState } from 'react'
import { Events } from '@wailsio/runtime'
import { Snapshot } from '../bindings/github.com/Aldzikra/cueflow/internal/app/service'
import type { ScriptInfo } from '../bindings/github.com/Aldzikra/cueflow/internal/app'
import type { AlignState } from '../bindings/github.com/Aldzikra/cueflow/internal/align'

export type Session = { script: ScriptInfo | null; state: AlignState | null; status: string }

// Mirrors backend state: one snapshot on mount, then events. No logic here.
export function useSession(): Session {
  const [s, setS] = useState<Session>({ script: null, state: null, status: 'idle' })
  useEffect(() => {
    let live = true
    Snapshot().then((snap) => {
      if (live && snap.script.text) setS({ script: snap.script, state: snap.state, status: snap.status })
    })
    const offs = [
      Events.On('script:loaded', (e) => setS((p) => ({ ...p, script: e.data as ScriptInfo }))),
      Events.On('align:state', (e) => setS((p) => ({ ...p, state: e.data as AlignState }))),
      Events.On('session:status', (e) => setS((p) => ({ ...p, status: e.data as string }))),
    ]
    return () => {
      live = false
      offs.forEach((off) => off())
    }
  }, [])
  return s
}
