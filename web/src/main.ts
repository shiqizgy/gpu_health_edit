import { createApp } from "vue";
import { createPinia } from "pinia";
import naive from "naive-ui";
import App from "./App.vue";
import router from "./router";
import "./styles.css";

declare const __APP_BUILD__: string; // 由 vite.config.ts 的 define 注入

const app = createApp(App);
app.use(createPinia());
app.use(router);
app.use(naive);
app.mount("#app");
console.info("[GPU HEALTH] 前端构建时间:", __APP_BUILD__);