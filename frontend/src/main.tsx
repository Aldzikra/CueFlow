import React from 'react'
import ReactDOM from 'react-dom/client'
import Editor from './Editor'
import Overlay from './Overlay'
import './style.css'

const isOverlay = window.location.hash === '#overlay'
document.body.classList.toggle('overlay-body', isOverlay)

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>{isOverlay ? <Overlay /> : <Editor />}</React.StrictMode>,
)
