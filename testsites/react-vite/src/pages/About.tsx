import { Link } from 'react-router-dom'

export default function About() {
  return (
    <section>
      <h1>About</h1>
      <p>
        This is a client-side routed page. Loading <code>/about</code> directly only works when the server
        falls back to <code>index.html</code> for unknown paths (history-mode SPA fallback).
      </p>
      <p>
        Built with Vite; the JavaScript, CSS and image assets are content-hashed files under{' '}
        <code>/assets/</code>.
      </p>
      <p>
        <Link to="/">Back home</Link>
      </p>
    </section>
  )
}
