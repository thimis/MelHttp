<script setup lang="ts">
import { useRoute } from 'vue-router'

const route = useRoute()
</script>

<template>
  <section>
    <h1>Page not found</h1>
    <p>
      Nothing lives at <code>{{ route.fullPath }}</code>.
    </p>
    <p><RouterLink to="/">Go home</RouterLink></p>
  </section>
</template>
