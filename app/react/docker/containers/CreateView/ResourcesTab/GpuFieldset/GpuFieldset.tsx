import { useMemo } from 'react';
import {
  ActionMeta,
  components,
  MultiValue,
  MultiValueRemoveProps,
  OnChangeValue,
  OptionProps,
} from 'react-select';
import { useTranslation } from 'react-i18next';

import { Select } from '@@/form-components/ReactSelect';
import { Switch } from '@@/form-components/SwitchField/Switch';
import { Tooltip } from '@@/Tip/Tooltip';
import { TextTip } from '@@/Tip/TextTip';

import { Values } from './types';

interface GpuOption {
  value: string;
  label: string;
  description?: string;
}

interface GPU {
  value: string;
  name: string;
}

export interface Props {
  values: Values;
  onChange(values: Values): void;
  gpus: GPU[];
  usedGpus: string[];
  usedAllGpus: boolean;
  enableGpuManagement?: boolean;
}

const NvidiaCapabilitiesOptions = [
  // Taken from https://github.com/containerd/containerd/blob/master/contrib/nvidia/nvidia.go#L40
  {
    value: 'compute',
    label: 'compute',
    description: 'required for CUDA and OpenCL applications',
  },
  {
    value: 'compat32',
    label: 'compat32',
    description: 'required for running 32-bit applications',
  },
  {
    value: 'graphics',
    label: 'graphics',
    description: 'required for running OpenGL and Vulkan applications',
  },
  {
    value: 'utility',
    label: 'utility',
    description: 'required for using nvidia-smi and NVML',
  },
  {
    value: 'video',
    label: 'video',
    description: 'required for using the Video Codec SDK',
  },
  {
    value: 'display',
    label: 'display',
    description: 'required for leveraging X11 display',
  },
] as const;

export function GpuFieldset({
  values,
  onChange,
  gpus = [],
  usedGpus = [],
  usedAllGpus,
  enableGpuManagement,
}: Props) {
  const { t } = useTranslation();
  const options = useMemo(() => {
    const options = (gpus || []).map((gpu) => ({
      value: gpu.value,
      label:
        usedGpus.includes(gpu.value) || usedAllGpus
          ? t('legacyText.{{name}} (in use)', {
              name: gpu.name,
              defaultValue: `${gpu.name} (in use)`,
            })
          : gpu.name,
    }));

    options.unshift({
      value: 'all',
      label: t('legacyText.Use All GPUs', { defaultValue: 'Use All GPUs' }),
    });

    return options;
  }, [gpus, t, usedGpus, usedAllGpus]);

  const gpuCmd = useMemo(() => {
    const devices = values.selectedGPUs.join(',');
    const deviceStr = devices === 'all' ? 'all,' : `device=${devices},`;
    const caps = values.capabilities.join(',');
    return `--gpus '${deviceStr}"capabilities=${caps}"'`;
  }, [values.selectedGPUs, values.capabilities]);

  const gpuValue = useMemo(
    () =>
      options.filter((option) => values.selectedGPUs.includes(option.value)),
    [values.selectedGPUs, options]
  );

  const capValue = useMemo(
    () =>
      NvidiaCapabilitiesOptions.filter((option) =>
        values.capabilities.includes(option.value)
      ),
    [values.capabilities]
  );

  return (
    <div>
      <TextTip inline={false} color="blue">
        <p>
          {t(
            'legacyText.GPU support is currently limited to NVIDIA graphics cards only.',
            {
              defaultValue:
                'GPU support is currently limited to NVIDIA graphics cards only.',
            }
          )}
        </p>
      </TextTip>

      {!enableGpuManagement && (
        <TextTip color="blue">
          {t(
            'legacyText.GPU in the UI is not currently enabled for this environment.',
            {
              defaultValue:
                'GPU in the UI is not currently enabled for this environment.',
            }
          )}
        </TextTip>
      )}

      <div className="form-group">
        <div className="col-sm-3 col-lg-2 control-label text-left">
          {t('legacyText.Enable GPU', { defaultValue: 'Enable GPU' })}
          <Switch
            id="enabled"
            name="enabled"
            checked={values.enabled && !!enableGpuManagement}
            onChange={toggleEnableGpu}
            className="ml-2"
            disabled={!enableGpuManagement}
            data-cy="docker-containers-gpu-enabled-switch"
          />
        </div>
        {enableGpuManagement && values.enabled && (
          <div className="col-sm-9 col-lg-10 text-left">
            <Select<GpuOption, true>
              isMulti
              closeMenuOnSelect
              value={gpuValue}
              isClearable={false}
              backspaceRemovesValue={false}
              isDisabled={!values.enabled}
              onChange={onChangeSelectedGpus}
              options={options}
              components={{ MultiValueRemove }}
              data-cy="docker-containers-gpu-select"
              id="docker-containers-gpu-select"
            />
          </div>
        )}
      </div>

      {values.enabled && (
        <>
          <div className="form-group">
            <div className="col-sm-3 col-lg-2 control-label text-left">
              {t('legacyText.Capabilities', {
                defaultValue: 'Capabilities',
              })}
              <Tooltip
                message={t(
                  'legacyText.compute and utility capabilities are preselected by Portainer because they are used by default when you do not explicitly specify capabilities with the docker CLI --gpus option.',
                  {
                    defaultValue:
                      'compute and utility capabilities are preselected by Portainer because they are used by default when you do not explicitly specify capabilities with the docker CLI --gpus option.',
                  }
                )}
              />
            </div>
            <div className="col-sm-9 col-lg-10 text-left">
              <Select<GpuOption, true>
                isMulti
                closeMenuOnSelect
                value={capValue}
                options={NvidiaCapabilitiesOptions}
                components={{ Option }}
                onChange={onChangeSelectedCaps}
                data-cy="docker-containers-gpu-capabilities-select"
                id="docker-containers-gpu-capabilities-select"
              />
            </div>
          </div>

          <div className="form-group">
            <div className="col-sm-3 col-lg-2 control-label text-left">
              {t('legacyText.Control', { defaultValue: 'Control' })}
              <Tooltip
                message={t(
                  'legacyText.This is the generated equivalent of the --gpus docker CLI parameter based on your settings.',
                  {
                    defaultValue:
                      'This is the generated equivalent of the --gpus docker CLI parameter based on your settings.',
                  }
                )}
              />
            </div>
            <div className="col-sm-9 col-lg-10">
              <code>{gpuCmd}</code>
            </div>
          </div>
        </>
      )}
    </div>
  );

  function onChangeValues(key: string, newValue: boolean | string[]) {
    const newValues = {
      ...values,
      [key]: newValue,
    };
    onChange(newValues);
  }

  function toggleEnableGpu() {
    onChangeValues('enabled', !values.enabled);
  }

  function onChangeSelectedGpus(
    newValue: OnChangeValue<GpuOption, true>,
    actionMeta: ActionMeta<GpuOption>
  ) {
    let { useSpecific } = values;
    let selectedGPUs = newValue.map((option) => option.value);

    if (actionMeta.action === 'select-option') {
      useSpecific = actionMeta.option?.value !== 'all';
      selectedGPUs = selectedGPUs.filter((value) =>
        useSpecific ? value !== 'all' : value === 'all'
      );
    }

    const newValues = { ...values, selectedGPUs, useSpecific };
    onChange(newValues);
  }

  function onChangeSelectedCaps(newValue: OnChangeValue<GpuOption, true>) {
    onChangeValues(
      'capabilities',
      newValue.map((option) => option.value)
    );
  }
}

function Option(props: OptionProps<GpuOption, true>) {
  const {
    data: { value, description },
  } = props;
  const { t } = useTranslation();
  const translatedDescription = description
    ? t(`legacyText.${description}`, { defaultValue: description })
    : '';

  return (
    <div>
      {/* eslint-disable-next-line react/jsx-props-no-spreading */}
      <components.Option {...props}>
        {translatedDescription ? `${value} - ${translatedDescription}` : value}
      </components.Option>
    </div>
  );
}

function MultiValueRemove(props: MultiValueRemoveProps<GpuOption, true>) {
  const {
    selectProps: { value },
  } = props;
  if (value && (value as MultiValue<GpuOption>).length === 1) {
    return null;
  }
  // eslint-disable-next-line react/jsx-props-no-spreading
  return <components.MultiValueRemove {...props} />;
}
