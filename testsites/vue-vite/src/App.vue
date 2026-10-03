<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import vueLogo from './assets/vue.svg'
</script>

<template>
  <div class="layout">
    <header class="topbar">
      <RouterLink to="/" class="brand">
        <img :src="vueLogo" alt="Vue logo" width="28" height="28" />
        <span>MelHttp · Vue</span>
      </RouterLink>
      <nav aria-label="Main">
        <RouterLink to="/">Home</RouterLink>
        <RouterLink to="/about">About</RouterLink>
      </nav>
    </header>
    <main class="content">
      <RouterView />
    </main>
    <footer class="footer">Served one Malbolge step at a time.</footer>
  </div>
</template>
