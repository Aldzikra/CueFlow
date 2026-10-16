import { useEffect, useState } from 'react'
import { Devices, Start, Stop, ToggleOverlay } from '../bindings/github.com/Aldzikra/cueflow/internal/app/service'
import type { Device } from '../bindings/github.com/Aldzikra/cueflow/internal/audio'
import { useSession } from './useSession'

export default function Editor() {
  const { state, status } = useSession()
  const [text, setText] = useState('')
  const [lang, setLang] = useState('id')
  const [devices, setDevices] = useState<Device[]>([])
  const [device, setDevice] = useState(-1)
  const [error, setError] = useState('')

  useEffect(() => {
    Devices().then((d) => setDevices(d ?? [])).catch((e) => setError(String(e)))
  }, [])

  const start = () => {
    setError('')
    Start(text, lang, device).catch((e) => setError(String(e?.message ?? e)))
  }

  return (
    <main className="editor">
      <h1>CueFlow</h1>
      <textarea value={text} onChange={(e) => setText(e.target.value)} placeholder="Paste your script here…" />
      <div className="row">
        <label>
          Language
          <select value={lang} onChange={(e) => setLang(e.target.value)}>
            <option value="id">Bahasa Indonesia</option>
            <option value="en">English</option>
          </select>
        </label>
        <label>
          Microphone
          <select value={device} onChange={(e) => setDevice(Number(e.target.value))}>
            <option value={-1}>System default</option>
            {devices.map((d) => (
              <option key={d.Index} value={d.Index}>
                {d.Name}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="row">
        <button onClick={start} disabled={!text.trim()}>
          Start
        </button>
        <button onClick={() => Stop()}>Stop</button>
        <button onClick={() => ToggleOverlay()}>Toggle overlay</button>
        <span className="status">
          {status}
          {state && ` · ${state.mode} · cursor ${state.cursor}`}
        </span>
      </div>
      {error && <p className="error">{error}</p>}
    </main>
  )
}
