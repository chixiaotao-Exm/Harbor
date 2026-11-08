import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import Workbench from './views/Workbench.vue'
import Settings from './views/Settings.vue'
import Accounts from './views/Accounts.vue'
import AsyncJobs from './views/AsyncJobs.vue'
import './style.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'work', component: Workbench, meta: { title: '工作台' } },
    { path: '/settings', name: 'settings', component: Settings, meta: { title: '设置' } },
    { path: '/accounts', name: 'accounts', component: Accounts, meta: { title: '账号' } },
    { path: '/async', name: 'async', component: AsyncJobs, meta: { title: '异步任务' } },
  ],
})

createApp(App).use(router).mount('#app')
