import { ref } from "vue";

/**
 * 统一的异步加载三态。
 * 解决的问题：原来请求失败时数据保持为空数组，
 * 页面渲染成 "No Data"，让人误以为是"确实没有数据"。
 */
export function useAsyncData<T>(initial: T) {
    const data = ref<T>(initial);
    const loading = ref(false);
    const error = ref<string>("");

    async function run(fn: () => Promise<T>) {
        loading.value = true;
        error.value = "";
        try {
            data.value = await fn();
        } catch (e: any) {
            error.value = e?.response?.data?.msg || e?.message || "请求失败";
        } finally {
            loading.value = false;
        }
    }
    return { data, loading, error, run };
}
