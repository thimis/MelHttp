<template>
  <section>
    <h1>About</h1>
    <p>
      This route is lazy-loaded and uses <code>createWebHistory</code>. Loading <code>/about</code> directly only
      works when the server falls back to <code>index.html</code> for unknown paths.
    </p>
    <p>
      Built with Vite; the JavaScript, CSS and image assets are content-hashed files under <code>/assets/</code>.
    </p>
    <p><RouterLink to="/">Back home</RouterLink></p>
  </section>
</template>
