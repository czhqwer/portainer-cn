package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	portainer "github.com/portainer/portainer/api"
)

type selectedConfigSet struct {
	configSet portainer.PlatformConfigSet
	all       bool
	keys      map[string]struct{}
}

type requiredConfigEntry struct {
	configSetID portainer.PlatformConfigSetID
	key         string
}

// BuildEffectiveConfigSnapshot resolves project, environment, and service-deployment
// configuration into the immutable release shape. 配置引用允许附加非 default 配置集，但
// 所有条目仍按 project < environment < service-deployment EnvOverrides 的固定优先级覆盖。
func BuildEffectiveConfigSnapshot(
	deployment portainer.PlatformServiceDeployment,
	configSets []portainer.PlatformConfigSet,
) (portainer.PlatformEffectiveConfigSnapshot, error) {
	selected, requiredEntries, err := selectConfigSets(deployment, configSets)
	if err != nil {
		return portainer.PlatformEffectiveConfigSnapshot{}, err
	}

	sort.Slice(selected, func(i, j int) bool {
		leftScope := configScopePriority(selected[i].configSet.ScopeType)
		rightScope := configScopePriority(selected[j].configSet.ScopeType)
		if leftScope != rightScope {
			return leftScope < rightScope
		}
		if selected[i].configSet.Name != selected[j].configSet.Name {
			return selected[i].configSet.Name < selected[j].configSet.Name
		}
		return selected[i].configSet.ID < selected[j].configSet.ID
	})

	entries := make(map[string]portainer.PlatformConfigEntrySnapshot)
	revisions := make(map[string]int, len(selected))
	foundRequiredEntries := make(map[requiredConfigEntry]bool, len(requiredEntries))
	for _, selection := range selected {
		configSet := selection.configSet
		revisions[configSetRevisionKey(configSet)] = configSet.Revision
		for _, entry := range configSet.Entries {
			if !selection.all {
				if _, selected := selection.keys[entry.Key]; !selected {
					continue
				}
			}

			entries[entry.Key] = configEntrySnapshot(entry)
			foundRequiredEntries[requiredConfigEntry{configSetID: configSet.ID, key: entry.Key}] = true
		}
	}

	for _, required := range requiredEntries {
		if !foundRequiredEntries[required] {
			return portainer.PlatformEffectiveConfigSnapshot{}, fmt.Errorf("required config entry %q is unavailable in config set %d", required.key, required.configSetID)
		}
	}

	// 服务部署级 EnvOverrides 始终是最后一层；它们不需要 ConfigSet 引用，才能保持
	// 阶段 1 已有部署表单的兼容性。敏感值只留下 hash 和存在性，不在快照响应中回传。
	for _, override := range deployment.DesiredSpec.EnvOverrides {
		if override.Name == "" {
			continue
		}
		entries[override.Name] = envOverrideSnapshot(override)
	}

	result := portainer.PlatformEffectiveConfigSnapshot{
		SpecRevision:       deployment.SpecRevision,
		ConfigSetRevisions: revisions,
		Entries:            sortedConfigEntries(entries),
	}
	result.Hash = effectiveConfigHash(result.Entries)

	return result, nil
}

// BuildSecretSnapshots selects the same winning entries as effective-config resolution,
// then retains only encrypted sensitive plain values for the immutable Release snapshot.
// 服务级旧 EnvOverrides 不能承载敏感明文；要求调用方改用加密 ConfigSet，避免新发布继续
// 扩散阶段 1 遗留的明文表达方式。
func BuildSecretSnapshots(
	deployment portainer.PlatformServiceDeployment,
	configSets []portainer.PlatformConfigSet,
) ([]portainer.PlatformSecretSnapshot, error) {
	selected, _, err := selectConfigSets(deployment, configSets)
	if err != nil {
		return nil, err
	}
	sort.Slice(selected, func(i, j int) bool {
		leftScope := configScopePriority(selected[i].configSet.ScopeType)
		rightScope := configScopePriority(selected[j].configSet.ScopeType)
		if leftScope != rightScope {
			return leftScope < rightScope
		}
		if selected[i].configSet.Name != selected[j].configSet.Name {
			return selected[i].configSet.Name < selected[j].configSet.Name
		}
		return selected[i].configSet.ID < selected[j].configSet.ID
	})

	winningEntries := make(map[string]portainer.PlatformConfigEntry)
	for _, selection := range selected {
		for _, entry := range selection.configSet.Entries {
			if !selection.all {
				if _, selected := selection.keys[entry.Key]; !selected {
					continue
				}
			}
			winningEntries[entry.Key] = entry
		}
	}
	for _, override := range deployment.DesiredSpec.EnvOverrides {
		if !override.IsSecret {
			continue
		}
		if override.Value != "" || override.HasValue {
			return nil, fmt.Errorf("sensitive EnvOverrides must be migrated to an encrypted config set")
		}
	}

	keys := make([]string, 0, len(winningEntries))
	for key := range winningEntries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]portainer.PlatformSecretSnapshot, 0)
	for _, key := range keys {
		entry := winningEntries[key]
		if !entry.Sensitive || entry.ValueType != portainer.PlatformConfigValuePlain {
			continue
		}
		if entry.CipherText == "" {
			if entry.Required {
				return nil, fmt.Errorf("required sensitive config entry %q has no encrypted value", entry.Key)
			}
			continue
		}
		if entry.EncryptionVersion == "" || entry.Hash == "" {
			return nil, fmt.Errorf("sensitive config entry %q has an invalid encrypted snapshot", entry.Key)
		}
		result = append(result, portainer.PlatformSecretSnapshot{
			Name:              entry.Key,
			CipherText:        entry.CipherText,
			EncryptionVersion: entry.EncryptionVersion,
			Hash:              entry.Hash,
			HasValue:          entry.HasValue,
		})
	}

	return result, nil
}

func selectConfigSets(
	deployment portainer.PlatformServiceDeployment,
	configSets []portainer.PlatformConfigSet,
) ([]selectedConfigSet, []requiredConfigEntry, error) {
	byID := make(map[portainer.PlatformConfigSetID]portainer.PlatformConfigSet, len(configSets))
	selected := make(map[portainer.PlatformConfigSetID]*selectedConfigSet)
	for _, configSet := range configSets {
		if configSet.LifecycleStatus != portainer.PlatformLifecycleStatusActive || configSet.ProjectID != deployment.ProjectID {
			continue
		}
		if !configSetMatchesDeployment(configSet, deployment) {
			continue
		}
		byID[configSet.ID] = configSet
		if configSet.Name == portainer.PlatformConfigSetDefaultName {
			selection := ensureConfigSetSelection(selected, configSet)
			selection.all = true
		}
	}

	requiredEntries := make([]requiredConfigEntry, 0)
	for _, reference := range deployment.DesiredSpec.ConfigRefs {
		if reference.ConfigSetID == 0 {
			if reference.Required {
				return nil, nil, fmt.Errorf("required config reference must include a config set ID")
			}
			continue
		}

		configSet, found := byID[reference.ConfigSetID]
		if !found {
			if reference.Required {
				return nil, nil, fmt.Errorf("required config set %d is unavailable", reference.ConfigSetID)
			}
			continue
		}

		selection := ensureConfigSetSelection(selected, configSet)
		if reference.Key == "" {
			selection.all = true
		} else {
			if selection.keys == nil {
				selection.keys = make(map[string]struct{})
			}
			selection.keys[reference.Key] = struct{}{}
			if reference.Required {
				requiredEntries = append(requiredEntries, requiredConfigEntry{configSetID: configSet.ID, key: reference.Key})
			}
		}
	}

	result := make([]selectedConfigSet, 0, len(selected))
	for _, selection := range selected {
		result = append(result, *selection)
	}
	return result, requiredEntries, nil
}

func ensureConfigSetSelection(selections map[portainer.PlatformConfigSetID]*selectedConfigSet, configSet portainer.PlatformConfigSet) *selectedConfigSet {
	selection, found := selections[configSet.ID]
	if !found {
		selection = &selectedConfigSet{configSet: configSet, keys: make(map[string]struct{})}
		selections[configSet.ID] = selection
	}
	return selection
}

func configSetMatchesDeployment(configSet portainer.PlatformConfigSet, deployment portainer.PlatformServiceDeployment) bool {
	switch configSet.ScopeType {
	case portainer.PlatformConfigScopeProject:
		return configSet.ScopeID == int(deployment.ProjectID)
	case portainer.PlatformConfigScopeEnvironment:
		return configSet.ScopeID == int(deployment.EnvironmentID)
	case portainer.PlatformConfigScopeServiceDeployment:
		return configSet.ScopeID == int(deployment.ID)
	default:
		return false
	}
}

func configScopePriority(scope portainer.PlatformConfigScopeType) int {
	switch scope {
	case portainer.PlatformConfigScopeProject:
		return 1
	case portainer.PlatformConfigScopeEnvironment:
		return 2
	case portainer.PlatformConfigScopeServiceDeployment:
		return 3
	default:
		return 4
	}
}

func configSetRevisionKey(configSet portainer.PlatformConfigSet) string {
	return fmt.Sprintf("%s:%d:%d", configSet.ScopeType, configSet.ScopeID, configSet.ID)
}

func configEntrySnapshot(entry portainer.PlatformConfigEntry) portainer.PlatformConfigEntrySnapshot {
	snapshot := portainer.PlatformConfigEntrySnapshot{
		Key:       entry.Key,
		ValueType: entry.ValueType,
		Sensitive: entry.Sensitive,
		Required:  entry.Required,
		Source:    entry.Source,
		HasValue:  entry.HasValue || entry.Value != "" || entry.CipherText != "",
	}
	if entry.Sensitive {
		snapshot.Hash = entry.Hash
		if snapshot.Hash == "" {
			snapshot.Hash = configValueHash(entry.Value)
		}
		return snapshot
	}

	snapshot.Value = entry.Value
	snapshot.Hash = entry.Hash
	if snapshot.Hash == "" {
		snapshot.Hash = configValueHash(entry.Value)
	}
	return snapshot
}

func envOverrideSnapshot(override portainer.PlatformEnvVar) portainer.PlatformConfigEntrySnapshot {
	snapshot := portainer.PlatformConfigEntrySnapshot{
		Key:       override.Name,
		ValueType: portainer.PlatformConfigValuePlain,
		Sensitive: override.IsSecret,
		Source:    portainer.PlatformConfigEntrySourceServiceDeployment,
		HasValue:  override.Value != "" || override.HasValue,
		Hash:      override.Hash,
	}
	if snapshot.Hash == "" {
		snapshot.Hash = configValueHash(override.Value)
	}
	if !override.IsSecret {
		snapshot.Value = override.Value
	}
	return snapshot
}

func sortedConfigEntries(entries map[string]portainer.PlatformConfigEntrySnapshot) []portainer.PlatformConfigEntrySnapshot {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]portainer.PlatformConfigEntrySnapshot, 0, len(keys))
	for _, key := range keys {
		result = append(result, entries[key])
	}
	return result
}

func effectiveConfigHash(entries []portainer.PlatformConfigEntrySnapshot) string {
	hash := sha256.New()
	for _, entry := range entries {
		value := entry.Value
		if entry.Sensitive {
			value = entry.Hash
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%t\x00%t\x00%s\x00%t\x00%s\n", entry.Key, entry.ValueType, entry.Sensitive, entry.Required, entry.Source, entry.HasValue, value)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func configValueHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
