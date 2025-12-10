<script setup>
const props = defineProps({
  commitSha: { type: String, default: 'dev' },
  builtAt: { type: String, default: '' },
})
</script>

<template>
  <footer class="footer">
    <div class="footer__container">
      <div class="footer__wrap">
        <p>
          &copy; {{ new Date().getFullYear() }} WaveYo.
          <a href="https://www.rainyun.com/WaveYo_" target="_blank" class="link">雨云提供计算支持</a> |
          EdgeOne提供CDN支持
        </p>
        <p>
          <a href="https://beian.miit.gov.cn/" target="_blank" class="link">皖ICP备2025078205号</a>&nbsp;&nbsp;
          <a href="https://beian.mps.gov.cn/#/query/webSearch?code=" rel="noreferrer" target="_blank" class="link">皖公网安备34040002000514号</a>
        </p>
      </div>
      <p class="footer__commit">
        Commit: {{ props.commitSha }} <span v-if="props.builtAt">| BuiltAt: {{ props.builtAt }}</span>
      </p>
    </div>
  </footer>
</template>

<style scoped>
@reference "../style.css";
.footer {
  @apply border-t;
  @apply border-gray-700;
  @apply py-8;
  @apply text-gray-500;
}

.footer__container {
  @apply container;
  @apply mx-auto;
  @apply px-4;
}

.footer__wrap {
  @apply flex;
  @apply flex-col;
  @apply sm:flex-row;
  @apply justify-between;
  @apply items-start;
  @apply sm:items-center;
  @apply gap-2;
}

.footer__commit {
  @apply mt-2;
  @apply text-xs;
  @apply text-gray-400;
  @apply font-mono;
}

.link {
  @apply hover:text-blue-400;
  @apply transition-colors;
  @apply duration-200;
}
</style>
