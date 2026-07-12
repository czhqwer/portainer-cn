import { useEffect, useRef } from 'react';

export function ArtifactTaskTerminal({
	logs,
	emptyMessage,
}: {
	logs?: string[];
	emptyMessage: string;
}) {
	const terminalRef = useRef<HTMLPreElement>(null);
	const content = logs?.length ? logs.join('\n') : emptyMessage;

	// 构建输出会随着轮询追加；始终跟随末尾让操作者不用反复手动滚动，
	// 同时仍保留原生滚动条以便回看已输出的 Docker 步骤。
	useEffect(() => {
		const terminal = terminalRef.current;
		if (terminal) {
			terminal.scrollTop = terminal.scrollHeight;
		}
	}, [content]);

	return (
		<pre
			ref={terminalRef}
			role="log"
			aria-live="polite"
			className="max-h-80 overflow-auto whitespace-pre-wrap break-words rounded border border-solid border-gray-6 bg-gray-11 p-3 font-mono text-xs leading-5 text-gray-1 th-highcontrast:border-white th-highcontrast:bg-black"
		>
			{content}
		</pre>
	);
}
