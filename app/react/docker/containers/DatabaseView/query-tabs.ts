export type QueryTabNameState = {
  name: string;
  defaultNameNumber?: number;
  isCustomName?: boolean;
};

export function normalizeQueryTabNames<T extends QueryTabNameState>(
  tabs: T[],
  formatDefaultName: (number: number) => string
) {
  const usedNumbers = new Set<number>();

  return tabs.map((tab) => {
    const automaticNumber = inferredDefaultNameNumber(tab);
    const nameNumber = numberInQueryTabName(tab.name);
    const isDuplicatedLegacyAutomaticTab =
      tab.isCustomName && !!nameNumber && usedNumbers.has(nameNumber);

    if (automaticNumber || isDuplicatedLegacyAutomaticTab) {
      const candidateNumber = automaticNumber || nameNumber!;
      const number = usedNumbers.has(candidateNumber)
        ? nextNumberAfterExisting(usedNumbers)
        : candidateNumber;
      usedNumbers.add(number);

      return {
        ...tab,
        name: formatDefaultName(number),
        defaultNameNumber: number,
        isCustomName: false,
      };
    }

    if (nameNumber) {
      usedNumbers.add(nameNumber);
    }

    return {
      ...tab,
      name: tab.name.trim(),
      isCustomName: true,
    };
  });
}

export function nextQueryTabNumber(tabs: QueryTabNameState[]) {
  const highestNumber = tabs.reduce((highest, tab) => {
    const number = tabNumberForAllocation(tab);
    return number ? Math.max(highest, number) : highest;
  }, 0);

  return highestNumber + 1;
}

export function appendAutomaticQueryTab<T extends QueryTabNameState>(
  tabs: T[],
  createTab: (number: number) => T
) {
  const tab = createTab(nextQueryTabNumber(tabs));

  return {
    tab,
    tabs: [...tabs, tab],
  };
}

function inferredDefaultNameNumber(tab: QueryTabNameState) {
  if (tab.isCustomName) {
    return undefined;
  }

  if (isValidTabNumber(tab.defaultNameNumber)) {
    return tab.defaultNameNumber;
  }

  return numberInQueryTabName(tab.name);
}

// 自动编号也要避开用户手动命名为“查询 N”的标签；
// 否则关闭标签后再新增，会出现两个可见名称相同的标签页。
function tabNumberForAllocation(tab: QueryTabNameState) {
  if (isValidTabNumber(tab.defaultNameNumber)) {
    return tab.defaultNameNumber;
  }

  return numberInQueryTabName(tab.name);
}

function numberInQueryTabName(name: string) {
  const match = name.trim().match(/^(?:Query|查询)\s*(\d+)$/i);
  const number = match ? Number(match[1]) : undefined;
  return isValidTabNumber(number) ? number : undefined;
}

function nextNumberAfterExisting(usedNumbers: Set<number>) {
  return Math.max(...usedNumbers) + 1;
}

function isValidTabNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0;
}
