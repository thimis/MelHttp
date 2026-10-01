import { useState } from 'react'

export default function Counter({ start = 0 }: { start?: number }) {
  const [count, setCount] = useState(start)

  return (
    <div className="counter">
      <button type="button" onClick={() => setCount((c) => c - 1)} aria-label="Decrement">
        −
      </button>
      <output aria-live="polite" data-testid="count">
        {count}
      </output>
      <button type="button" onClick={() => setCount((c) => c + 1)} aria-label="Increment">
        +
      </button>
    </div>
  )
}
