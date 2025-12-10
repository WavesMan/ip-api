<script setup>
import { ref, onMounted } from 'vue'
import HeaderNav from './components/HeaderNav.vue'
import FooterBar from './components/FooterBar.vue'

const commitSha = ref(typeof window !== 'undefined' ? (window.__COMMIT_SHA__ || 'dev') : 'dev')
const builtAt = ref('')

async function fetchVersion() {
  try {
    const base = window.__API_BASE__ || '/api'
    const res = await fetch(`${base}/version`)
    if (!res.ok) return
    const data = await res.json()
    if (data && data.commit) commitSha.value = data.commit
    if (data && data.builtAt) builtAt.value = data.builtAt
  } catch {}
}

onMounted(() => { fetchVersion() })
</script>

<template>
  <div class="app-root">
    <HeaderNav />
    <RouterView />
    <FooterBar :commit-sha="commitSha" :built-at="builtAt" />
  </div>
</template>

<style scoped>
@reference "./style.css";
.app-root {
  @apply min-h-screen;
  @apply flex;
  @apply flex-col;
  @apply bg-[#1a1a1a];
  @apply text-[#e5e7eb];
}
</style>
