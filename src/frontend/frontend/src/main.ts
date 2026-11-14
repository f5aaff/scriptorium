import './style.css'
import App from './App.svelte'

const target = document.getElementById('app')

if (!target) {
  throw new Error('Could not find application mount element: #app')
}

const app = new App({
  target
})

export default app
