import heroImg from '../assets/hero.png'
import Counter from '../components/Counter.tsx'

const OPCODES = [
  { code: 'j', name: 'mov d, [d]', note: 'jump the data pointer' },
  { code: 'i', name: 'jmp [d]', note: 'jump the code pointer' },
  { code: '*', name: 'rotr', note: 'rotate a trit word right' },
  { code: 'p', name: 'crz', note: 'the tritwise crazy operation' },
  { code: '<', name: 'out', note: 'print A mod 256' },
  { code: '/', name: 'in', note: 'read one character' },
  { code: 'v', name: 'hlt', note: 'stop the machine' },
  { code: 'o', name: 'nop', note: 'do nothing at all' },
]

export default function Home() {
  return (
    <>
      <section className="hero">
        <img src={heroImg} alt="Stacked layers illustration" width="170" height="179" data-testid="hero-image" />
        <div>
          <h1>React on MelHttp</h1>
          <p>
            A tiny React + Vite + react-router app. Every byte you are reading was emitted by a Malbolge
            program running inside the MelHttp server.
          </p>
        </div>
      </section>

      <section>
        <h2>Counter</h2>
        <Counter />
      </section>

      <section>
        <h2>The eight instructions</h2>
        <ul className="ops" aria-label="Malbolge instructions">
          {OPCODES.map((op) => (
            <li key={op.code}>
              <code>{op.code}</code> <strong>{op.name}</strong> — {op.note}
            </li>
          ))}
        </ul>
      </section>
    </>
  )
}
