<script setup lang="ts">
import { computed, ref } from 'vue'

const props = withDefaults(defineProps<{ start?: number }>(), { start: 0 })
const count = ref(props.start)
/** The count in base 3, because this is a Malbolge site. */
const ternary = computed(() => (count.value < 0 ? '-' : '') + Math.abs(count.value).toString(3))
</script>

<template>
  <div class="counter">
    <button type="button" aria-label="Decrement" @click="count--">−</button>
    <output aria-live="polite" data-testid="count">{{ count }}</output>
    <button type="button" aria-label="Increment" @click="count++">+</button>
    <span class="ternary">base 3: <code data-testid="ternary">{{ ternary }}</code></span>
  </div>
</template>
