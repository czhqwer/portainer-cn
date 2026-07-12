import { render, screen } from '@testing-library/react';

import { ArtifactTaskTerminal } from './ArtifactTaskTerminal';

test('renders persisted artifact build output in a terminal log', () => {
	render(
		<ArtifactTaskTerminal
			logs={['Step 1/2 : FROM eclipse-temurin:8-jre']}
			emptyMessage="Waiting for build output"
		/>
	);

	expect(screen.getByRole('log')).toHaveTextContent(
		'Step 1/2 : FROM eclipse-temurin:8-jre'
	);
});
