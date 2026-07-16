<!--
  LifeListImportField.svelte

  Purpose: Upload an eBird life-list CSV export (or add species manually),
  parse it client-side, and manage the resulting list of "on my life list"
  species entries. Unlike SpeciesListCard, entries here are not validated
  against BirdNET's own species catalog (an eBird export can legitimately
  contain species BirdNET doesn't classify) — matching against detections
  happens continuously on the backend by scientific name, not at import time.

  Props:
  - species: string[] - Current life-list entries ("ScientificName_CommonName" or scientific-name-only)
  - disabled: boolean - Disable all interactions
  - onImport: (entries: string[]) => void - Called with newly parsed entries to merge in
  - onRemove: (entry: string) => void - Remove a single entry
  - onAdd: (entry: string) => void - Add a single manually-typed entry

  @component
-->
<script lang="ts">
  import { Trash2, Search, Plus, Upload, TriangleAlert } from '@lucide/svelte';
  import { t } from '$lib/i18n';
  import { toastActions } from '$lib/stores/toast';
  import { readFileAsText } from '$lib/utils/fileHelpers';
  import { parseLifeListCsv, type LifeListParseRejection } from '$lib/utils/lifeListCsv';
  import ResizableContainer from '$lib/desktop/components/ui/ResizableContainer.svelte';

  interface Props {
    species: string[];
    disabled?: boolean;
    onImport: (_entries: string[]) => void;
    onRemove: (_entry: string) => void;
    onAdd: (_entry: string) => void;
  }

  let { species, disabled = false, onImport, onRemove, onAdd }: Props = $props();

  let fileInput: HTMLInputElement;
  let manualInputValue = $state('');
  let searchQuery = $state('');
  let showSearch = $derived(species.length > 8);

  // Last import's summary, cleared when the user dismisses it or imports again.
  let importSummary = $state<{
    acceptedCount: number;
    totalRows: number;
    rejected: LifeListParseRejection[];
  } | null>(null);
  let showRejectedRows = $state(false);

  function splitEntry(entry: string): { scientificName: string; commonName: string } {
    const idx = entry.indexOf('_');
    if (idx === -1) return { scientificName: entry, commonName: '' };
    return { scientificName: entry.slice(0, idx), commonName: entry.slice(idx + 1) };
  }

  let filteredSpecies = $derived.by(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return species;
    return species.filter(entry => entry.toLowerCase().includes(query));
  });

  function triggerFileBrowse() {
    if (!disabled) fileInput?.click();
  }

  async function handleFileSelect(event: Event) {
    if (disabled) return;
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    try {
      const text = await readFileAsText(file, 5_242_880); // 5MB — generous for even a large life list
      const result = parseLifeListCsv(text);

      if (result.accepted.length === 0 && result.rejected.length === 0) {
        toastActions.error(t('settings.species.lifeList.emptyFile'));
        return;
      }

      importSummary = {
        acceptedCount: result.accepted.length,
        totalRows: result.accepted.length + result.rejected.length,
        rejected: result.rejected,
      };
      showRejectedRows = false;

      if (result.accepted.length > 0) {
        onImport(result.accepted);
      }
    } catch (err) {
      toastActions.error(
        err instanceof Error ? err.message : t('settings.species.lifeList.fileReadError')
      );
    } finally {
      input.value = '';
    }
  }

  function handleManualAdd() {
    const trimmed = manualInputValue.trim();
    if (!trimmed || disabled) return;
    onAdd(trimmed);
    manualInputValue = '';
  }

  function handleManualKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleManualAdd();
    }
  }
</script>

<div class="bg-[var(--surface-100)] border border-[var(--border-100)] rounded-xl shadow-sm">
  <!-- Header -->
  <div class="flex items-center justify-between px-4 py-3 border-b border-[var(--border-100)]">
    <div class="flex items-center gap-2">
      <div class="p-1.5 rounded-lg bg-[color-mix(in_srgb,var(--color-success)_10%,transparent)]">
        <Upload class="w-4 h-4 text-[var(--color-success)]" />
      </div>
      <h3 class="text-xs font-semibold uppercase tracking-wider text-muted">
        {t('settings.species.lifeList.title')}
      </h3>
      <span
        class="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-500/10 text-muted"
      >
        {species.length}
      </span>
    </div>

    {#if showSearch}
      <div class="relative">
        <Search
          class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted"
          aria-hidden="true"
        />
        <input
          type="text"
          bind:value={searchQuery}
          placeholder={t('settings.species.activeSpecies.search.placeholder')}
          aria-label={t('settings.species.activeSpecies.search.placeholder')}
          autocomplete="off"
          data-1p-ignore
          data-lpignore="true"
          data-form-type="other"
          class="w-48 pl-8 pr-3 py-1.5 text-xs rounded-lg border border-[var(--border-100)] bg-[var(--surface-100)] focus:outline-none focus:ring-2 focus:ring-blue-500/40"
        />
      </div>
    {/if}
  </div>

  <!-- Import summary, shown after a CSV import until dismissed -->
  {#if importSummary}
    <div class="px-4 py-3 border-b border-[var(--border-100)] bg-[var(--color-base-200)] text-sm">
      <p>
        {t('settings.species.lifeList.importSummary', {
          accepted: importSummary.acceptedCount,
          total: importSummary.totalRows,
        })}
      </p>
      {#if importSummary.rejected.length > 0}
        <button
          type="button"
          class="mt-1 text-xs text-[var(--color-warning)] underline cursor-pointer"
          onclick={() => (showRejectedRows = !showRejectedRows)}
        >
          {showRejectedRows
            ? t('settings.species.lifeList.hideRejectedRows')
            : t('settings.species.lifeList.showRejectedRows', {
                count: importSummary.rejected.length,
              })}
        </button>
        {#if showRejectedRows}
          <ul class="mt-2 space-y-1 text-xs text-muted max-h-40 overflow-y-auto">
            {#each importSummary.rejected as rejection (rejection.rowNumber)}
              <li class="flex items-start gap-1.5">
                <TriangleAlert class="w-3 h-3 shrink-0 mt-0.5 text-[var(--color-warning)]" />
                <span>
                  {t('settings.species.lifeList.rejectedRow', { row: rejection.rowNumber })}: {rejection.reason}
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
      <button
        type="button"
        class="mt-2 text-xs text-muted underline cursor-pointer"
        onclick={() => (importSummary = null)}
      >
        {t('common.dismiss')}
      </button>
    </div>
  {/if}

  <!-- Species List -->
  {#if species.length > 0}
    <ResizableContainer defaultHeight={448} minHeight={200} maxHeight={800}>
      <table class="w-full text-sm">
        <thead class="sticky top-0 bg-[var(--surface-100)] z-10">
          <tr class="border-b border-[var(--border-100)]">
            <th class="text-left py-2 px-3 text-xs font-medium text-muted">
              {t('settings.species.activeSpecies.columns.commonName')}
            </th>
            <th class="text-left py-2 px-3 text-xs font-medium text-muted">
              {t('settings.species.activeSpecies.columns.scientificName')}
            </th>
            <th class="w-12"><span class="sr-only">{t('common.actionsColumn')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each filteredSpecies as entry, index (`${entry}_${index}`)}
            {@const parts = splitEntry(entry)}
            <tr
              class="border-b last:border-b-0 border-[var(--border-100)]/50 hover:bg-black/[0.02] dark:hover:bg-white/[0.02] transition-colors"
            >
              <td class="py-2 px-3">
                <span class="font-medium text-sm">{parts.commonName || '—'}</span>
              </td>
              <td class="py-2 px-3">
                <span class="text-xs text-muted italic">{parts.scientificName}</span>
              </td>
              <td class="py-2 px-3 w-12 text-right">
                <button
                  type="button"
                  class="p-1.5 rounded-md transition-colors cursor-pointer hover:bg-[color-mix(in_srgb,var(--color-error)_10%,transparent)] text-muted hover:text-[var(--color-error)] disabled:opacity-50 disabled:cursor-not-allowed"
                  onclick={() => onRemove(entry)}
                  {disabled}
                  aria-label={t('settings.species.remove')}
                  title={t('settings.species.remove')}
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
      {#if searchQuery && filteredSpecies.length === 0}
        <div class="text-sm text-muted italic py-6 text-center">
          {t('settings.species.activeSpecies.noResults')}
        </div>
      {/if}
    </ResizableContainer>
  {:else}
    <div class="text-sm text-muted italic py-8 text-center">
      {t('settings.species.lifeList.emptyMessage')}
    </div>
  {/if}

  <!-- Import / Add controls -->
  <div class="px-4 py-3 border-t border-[var(--border-100)] space-y-3">
    <div>
      <span class="text-xs font-semibold uppercase tracking-wider text-muted mb-2 block">
        {t('settings.species.lifeList.importLabel')}
      </span>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 px-3.5 py-2 text-sm font-medium rounded-lg bg-[var(--color-base-200)] border border-[var(--color-base-300)] transition-all disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer hover:bg-[var(--color-base-300)]"
        {disabled}
        onclick={triggerFileBrowse}
      >
        <Upload class="w-4 h-4" aria-hidden="true" />
        {t('settings.species.lifeList.uploadCsv')}
      </button>
      <input
        bind:this={fileInput}
        type="file"
        accept=".csv"
        class="hidden"
        onchange={handleFileSelect}
        tabindex={-1}
      />
      <p class="mt-1.5 text-xs text-muted">{t('settings.species.lifeList.uploadHelp')}</p>
    </div>

    <div>
      <label
        for="life-list-manual-input"
        class="text-xs font-semibold uppercase tracking-wider text-muted mb-2 block"
      >
        {t('settings.species.lifeList.addManuallyLabel')}
      </label>
      <div class="flex gap-2">
        <input
          id="life-list-manual-input"
          type="text"
          bind:value={manualInputValue}
          onkeydown={handleManualKeydown}
          placeholder={t('settings.species.lifeList.addManuallyPlaceholder')}
          {disabled}
          autocomplete="off"
          data-1p-ignore
          data-lpignore="true"
          data-form-type="other"
          class="flex-1 px-3 py-2 text-sm rounded-lg border border-[var(--border-100)] bg-[var(--surface-100)] focus:outline-none focus:ring-2 focus:ring-blue-500/40 disabled:opacity-50 disabled:cursor-not-allowed"
        />
        <button
          type="button"
          onclick={handleManualAdd}
          disabled={disabled || !manualInputValue.trim()}
          class="inline-flex items-center gap-1.5 px-3.5 py-2 text-sm font-medium rounded-lg bg-primary text-[var(--color-primary-content)] hover:bg-[var(--color-primary-hover)] transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          aria-label={t('settings.species.lifeList.addManuallyLabel')}
        >
          <Plus class="w-4 h-4" aria-hidden="true" />
          {t('settings.species.add')}
        </button>
      </div>
    </div>
  </div>
</div>
