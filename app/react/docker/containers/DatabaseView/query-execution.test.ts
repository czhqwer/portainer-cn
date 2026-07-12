import { describe, expect, it } from 'vitest';

import {
  normalizeEditorLineEndings,
  selectedOrAllEditorQuery,
} from './query-execution';

describe('selectedOrAllEditorQuery', () => {
  it('uses the textarea value when selecting a command after Windows line endings', () => {
    const persistedDraft = 'GET laoshi:1\r\nGET laoshi:2';
    const editorValue = normalizeEditorLineEndings(persistedDraft);
    const secondCommandStart = editorValue.indexOf('GET laoshi:2');

    expect(
      selectedOrAllEditorQuery(
        editorValue,
        secondCommandStart,
        editorValue.length
      )
    ).toBe('GET laoshi:2');
  });
});
