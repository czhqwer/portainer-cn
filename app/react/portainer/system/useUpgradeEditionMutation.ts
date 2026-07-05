import { useMutation } from '@tanstack/react-query';

export function useUpgradeEditionMutation() {
  return useMutation(upgradeEdition);
}

async function upgradeEdition(_: { license: string }) {
  return undefined;
}
