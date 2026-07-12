import { describe, expect, it } from 'vitest';

import {
  appendAutomaticQueryTab,
  nextQueryTabNumber,
  normalizeQueryTabNames,
} from './query-tabs';

describe('normalizeQueryTabNames', () => {
  const formatName = (number: number) => `查询 ${number}`;

  it('migrates legacy English default names to the current language', () => {
    const [tab] = normalizeQueryTabNames(
      [{ id: '1', name: 'Query 1' }],
      formatName
    );

    expect(tab).toMatchObject({
      name: '查询 1',
      defaultNameNumber: 1,
      isCustomName: false,
    });
  });

  it('assigns a unique default number when persisted tabs are duplicated', () => {
    const tabs = normalizeQueryTabNames(
      [
        { id: '1', name: '查询 2' },
        { id: '2', name: '查询 2' },
      ],
      formatName
    );

    expect(tabs.map((tab) => tab.name)).toEqual(['查询 2', '查询 3']);
  });

  it('repairs a duplicated automatic label persisted as a custom tab', () => {
    const tabs = normalizeQueryTabNames(
      [
        { id: '1', name: 'Query 3' },
        { id: '2', name: 'Query 3', isCustomName: true },
      ],
      formatName
    );

    expect(tabs.map((tab) => tab.name)).toEqual(['查询 3', '查询 4']);
  });

  it('keeps an actual custom name when its previous automatic number is in use', () => {
    const tabs = normalizeQueryTabNames(
      [
        { id: '1', name: 'Query 3' },
        {
          id: '2',
          name: 'Monthly report',
          defaultNameNumber: 3,
          isCustomName: true,
        },
      ],
      formatName
    );

    expect(tabs.map((tab) => tab.name)).toEqual(['查询 3', 'Monthly report']);
    expect(tabs[1]).toMatchObject({ isCustomName: true });
  });
});

describe('nextQueryTabNumber', () => {
  it('continues after the highest active automatic tab number', () => {
    expect(
      nextQueryTabNumber([
        { name: '查询 1', defaultNameNumber: 1 },
        { name: '查询 3', defaultNameNumber: 3 },
      ])
    ).toBe(4);
  });

  it('does not reuse a number that is already used by a custom tab label', () => {
    expect(
      nextQueryTabNumber([
        { name: 'Query 2', defaultNameNumber: 2 },
        { name: 'Query 3', isCustomName: true },
      ])
    ).toBe(4);
  });
});

describe('appendAutomaticQueryTab', () => {
  it('allocates unique numbers from the latest committed tabs', () => {
    const createTab = (number: number) => ({
      name: `Query ${number}`,
      defaultNameNumber: number,
    });
    const existingTabs = [
      { name: 'Query 2', defaultNameNumber: 2 },
      { name: 'Query 3', defaultNameNumber: 3 },
    ];

    const firstAppend = appendAutomaticQueryTab(existingTabs, createTab);
    const secondAppend = appendAutomaticQueryTab(firstAppend.tabs, createTab);

    expect(secondAppend.tabs.map((tab) => tab.name)).toEqual([
      'Query 2',
      'Query 3',
      'Query 4',
      'Query 5',
    ]);
  });
});
