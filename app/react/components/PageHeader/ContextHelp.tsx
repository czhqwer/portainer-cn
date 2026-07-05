import { useCurrentStateAndParams } from '@uirouter/react';

import { useSystemVersion } from '@/react/portainer/system/useSystemVersion';

export function ContextHelp() {
  return null;
}

export function useDocsUrl(doc?: string): string {
  const { state } = useCurrentStateAndParams();
  const versionQuery = useSystemVersion();

  if (!doc && !state) {
    return '';
  }

  let url = 'https://docs.portainer.io/';

  // Add LTS or STS version if we have it
  if (versionQuery.data?.VersionSupport) {
    url += versionQuery.data.VersionSupport.toLowerCase();
  }

  if (doc) {
    return url + doc;
  }

  const { data } = state;
  if (
    data &&
    typeof data === 'object' &&
    'docs' in data &&
    typeof data.docs === 'string'
  ) {
    return url + data.docs;
  }

  return url;
}
