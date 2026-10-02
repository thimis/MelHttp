import { NavLink, Route, Routes } from 'react-router-dom'
import reactLogo from './assets/react.svg'
import Home from './pages/Home.tsx'
import About from './pages/About.tsx'
import NotFound from './pages/NotFound.tsx'

export default function App() {
  return (
    <div className="layout">
      <header className="topbar">
        <NavLink to="/" className="brand" end>
          <img src={reactLogo} alt="React logo" width="28" height="28" />
          <span>MelHttp · React</span>
        </NavLink>
        <nav aria-label="Main">
          <NavLink to="/" end>
            Home
          </NavLink>
          <NavLink to="/about">About</NavLink>
        </nav>
      </header>
      <main className="content">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/about" element={<About />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>
      <footer className="footer">Served one Malbolge step at a time.</footer>
    </div>
  )
}
