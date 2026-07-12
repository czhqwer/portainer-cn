// 制品页的 mutation 已由全局 MutationCache 转换为用户可见的错误提示；
// 这里消化 Promise 拒绝，避免表单事件处理器再次把同一错误抛到开发环境遮罩层。
export async function runArtifactOperation(
	operation: () => Promise<unknown>,
	onSuccess: () => void,
	onFinished?: () => void
) {
	try {
		await operation();
		onSuccess();
		return true;
	} catch {
		return false;
	} finally {
		onFinished?.();
	}
}
