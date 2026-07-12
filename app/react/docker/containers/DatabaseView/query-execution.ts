export function normalizeEditorLineEndings(value: string) {
  return value.replace(/\r\n?/g, '\n');
}

export function selectedOrAllEditorQuery(
  editorValue: string,
  selectionStart: number,
  selectionEnd: number
) {
  const selected = editorValue.slice(selectionStart, selectionEnd);
  return selected.trim() ? selected : editorValue;
}
