import { runArtifactOperation } from './artifactOperation';

test('keeps the artifact panel open when a mutation fails', async () => {
	const onSuccess = vi.fn();
	const onFinished = vi.fn();

	const succeeded = await runArtifactOperation(
		async () => Promise.reject(new Error('Java build failed')),
		onSuccess,
		onFinished
	);

	expect(succeeded).toBe(false);
	expect(onSuccess).not.toHaveBeenCalled();
	expect(onFinished).toHaveBeenCalledTimes(1);
});
