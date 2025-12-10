import { createRouter, createWebHashHistory } from 'vue-router'
import HomePage from '../pages/HomePage.vue'
import DocsPage from '../pages/DocsPage.vue'

const routes = [
  { path: '/', redirect: '/home' },
  { path: '/home', component: HomePage },
  { path: '/docs', component: DocsPage },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

