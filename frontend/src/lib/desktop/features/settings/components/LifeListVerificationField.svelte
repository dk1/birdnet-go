<!--
  LifeListVerificationField.svelte

  Purpose: Configure the second-model check a lifer must pass before its alert
  is sent. The backend re-reads the detection's audio and scores it with the
  chosen model; the alert fires only when that model also hears the species at
  the minimum confidence or above. Only enabled bird and wildlife models are offered, and the
  model must stay enabled under the analysis settings to be loaded, although it
  need not run on any audio source.

  Props:
  - settings: LiferVerificationSettings - Current verification settings
  - disabled: boolean - Disable all interactions
  - onChange: (settings: LiferVerificationSettings) => void - Called with the updated settings

  @component
-->
<script lang="ts">
  import { t } from '$lib/i18n';
  import Checkbox from '$lib/desktop/components/forms/Checkbox.svelte';
  import NumberField from '$lib/desktop/components/forms/NumberField.svelte';
  import SelectDropdown from '$lib/desktop/components/forms/SelectDropdown.svelte';
  import type { SelectOption } from '$lib/desktop/components/forms/SelectDropdown.types';
  import SettingsNote from '$lib/desktop/features/settings/components/SettingsNote.svelte';
  import { fetchModels, getAvailableModels } from '$lib/stores/models.svelte';
  import {
    LIFER_VERIFICATION_DEFAULT_MIN_CONFIDENCE,
    type LiferVerificationSettings,
  } from '$lib/stores/settings';

  interface Props {
    settings: LiferVerificationSettings;
    disabled?: boolean;
    onChange: (_settings: LiferVerificationSettings) => void;
  }

  let { settings, disabled = false, onChange }: Props = $props();

  $effect(() => {
    return fetchModels();
  });

  // Models that classify birds: the bird-only ones and the wildlife ones (Perch),
  // which cover birds among other taxa. Bat and geomodel entries cannot verify a bird.
  const VERIFIER_CATEGORIES = new Set(['bird', 'wildlife']);

  let modelOptions = $derived(
    getAvailableModels()
      .filter(m => VERIFIER_CATEGORIES.has(m.category) && m.state !== 'failed')
      .map(m => ({ value: m.id, label: m.name })) as SelectOption[]
  );

  // Shown as a percentage, stored on the 0 to 1 scale; 0 means the server default.
  let minConfidencePercent = $derived(
    Math.round((settings.minConfidence || LIFER_VERIFICATION_DEFAULT_MIN_CONFIDENCE) * 100)
  );

  function update(patch: Partial<LiferVerificationSettings>) {
    onChange({ ...settings, ...patch });
  }
</script>

<div class="space-y-4">
  <Checkbox
    checked={settings.enabled}
    label={t('settings.species.lifeList.verification.enableLabel')}
    helpText={t('settings.species.lifeList.verification.enableHelp')}
    {disabled}
    onchange={enabled => update({ enabled })}
  />

  {#if settings.enabled}
    {#if modelOptions.length < 2}
      <SettingsNote>
        {t('settings.species.lifeList.verification.needSecondModel')}
      </SettingsNote>
    {/if}

    <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
      <SelectDropdown
        id="lifer-verification-model"
        label={t('settings.species.lifeList.verification.modelLabel')}
        helpText={t('settings.species.lifeList.verification.modelHelp')}
        placeholder={t('settings.species.lifeList.verification.modelPlaceholder')}
        value={settings.model}
        options={modelOptions}
        {disabled}
        menuSize="sm"
        onChange={value => update({ model: String(value) })}
      />

      <NumberField
        label={t('settings.species.lifeList.verification.minConfidenceLabel')}
        helpText={t('settings.species.lifeList.verification.minConfidenceHelp')}
        value={minConfidencePercent}
        min={1}
        max={100}
        step={1}
        {disabled}
        onUpdate={value => update({ minConfidence: value / 100 })}
      />
    </div>

    <Checkbox
      checked={settings.notifyRejected}
      label={t('settings.species.lifeList.verification.notifyRejectedLabel')}
      helpText={t('settings.species.lifeList.verification.notifyRejectedHelp')}
      {disabled}
      onchange={notifyRejected => update({ notifyRejected })}
    />
  {/if}
</div>
