import { Link, useLocation } from 'react-router-dom'

export default function NotFound() {
  const { pathname } = useLocation()
  return (
    <section>
      <h1>Page not found</h1>
      <p>
        Nothing lives at <code>{pathname}</code>.
      </p>
      <p>
        <Link to="/">Go home</Link>
      </p>
    </section>
  )
}
