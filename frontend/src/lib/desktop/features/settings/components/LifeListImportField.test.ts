import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import LifeListImportField from './LifeListImportField.svelte';

// This test suite matches literal i18n key strings rather than translated
// English text, following this codebase's convention (t() returns the raw
// key in the test environment; see SpeciesConfigTable.test.ts).
const KEY_EMPTY_MESSAGE = 'settings.species.lifeList.emptyMessage';
const KEY_ADD_MANUALLY_LABEL = 'settings.species.lifeList.addManuallyLabel';
const KEY_UPLOAD_CSV = 'settings.species.lifeList.uploadCsv';
const KEY_REMOVE = 'settings.species.remove';
const KEY_DISMISS = 'common.dismiss';
const KEY_SEARCH_PLACEHOLDER = 'settings.species.activeSpecies.search.placeholder';
const KEY_SHOW_REJECTED_ROWS = 'settings.species.lifeList.showRejectedRows';
const KEY_IMPORT_SUMMARY = 'settings.species.lifeList.importSummary';

function renderField(overrides: Record<string, unknown> = {}) {
  const onImport = vi.fn();
  const onRemove = vi.fn();
  const onAdd = vi.fn();
  render(LifeListImportField, {
    props: {
      species: [],
      onImport,
      onRemove,
      onAdd,
      ...overrides,
    },
  });
  return { onImport, onRemove, onAdd };
}

describe('LifeListImportField', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows the empty message when the list is empty', () => {
    renderField();
    expect(screen.getByText(KEY_EMPTY_MESSAGE)).toBeInTheDocument();
  });

  it('renders existing entries split into common and scientific name columns', () => {
    renderField({ species: ['Turdus merula_Common Blackbird', 'Parus major'] });

    expect(screen.getByText('Common Blackbird')).toBeInTheDocument();
    expect(screen.getByText('Turdus merula')).toBeInTheDocument();
    // Scientific-name-only entry: common name column falls back to an em dash.
    expect(screen.getByText('Parus major')).toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('shows the current entry count badge', () => {
    renderField({ species: ['Turdus merula_Common Blackbird', 'Parus major_Great Tit'] });
    expect(screen.getByText('2')).toBeInTheDocument();
  });

  it('adds a manually-typed entry and clears the input', async () => {
    const { onAdd } = renderField();
    const user = userEvent.setup();

    const input = screen.getByRole('textbox', { name: KEY_ADD_MANUALLY_LABEL });
    await user.type(input, 'Turdus merula');
    await user.click(screen.getByRole('button', { name: KEY_ADD_MANUALLY_LABEL }));

    expect(onAdd).toHaveBeenCalledWith('Turdus merula');
    expect(input).toHaveValue('');
  });

  it('adds a manually-typed entry on Enter', async () => {
    const { onAdd } = renderField();
    const user = userEvent.setup();

    const input = screen.getByRole('textbox', { name: KEY_ADD_MANUALLY_LABEL });
    await user.type(input, 'Turdus merula{Enter}');

    expect(onAdd).toHaveBeenCalledWith('Turdus merula');
  });

  it('does not call onAdd for blank/whitespace-only input', async () => {
    const { onAdd } = renderField();
    const user = userEvent.setup();

    const input = screen.getByRole('textbox', { name: KEY_ADD_MANUALLY_LABEL });
    await user.type(input, '   ');
    const addButton = screen.getByRole('button', { name: KEY_ADD_MANUALLY_LABEL });
    expect(addButton).toBeDisabled();

    expect(onAdd).not.toHaveBeenCalled();
  });

  it('calls onRemove with the entry when its remove button is clicked', async () => {
    const { onRemove } = renderField({ species: ['Turdus merula_Common Blackbird'] });
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: KEY_REMOVE }));

    expect(onRemove).toHaveBeenCalledWith('Turdus merula_Common Blackbird');
  });

  it('disables interactive controls when disabled is true', () => {
    renderField({ species: ['Turdus merula_Common Blackbird'], disabled: true });

    expect(screen.getByRole('textbox', { name: KEY_ADD_MANUALLY_LABEL })).toBeDisabled();
    expect(screen.getByRole('button', { name: KEY_UPLOAD_CSV })).toBeDisabled();
    expect(screen.getByRole('button', { name: KEY_REMOVE })).toBeDisabled();
  });

  it('parses an uploaded CSV and calls onImport with accepted entries', async () => {
    const { onImport } = renderField();
    const user = userEvent.setup();

    const csv = 'Common Name,Scientific Name\nCommon Blackbird,Turdus merula\n';
    const file = new File([csv], 'life-list.csv', { type: 'text/csv' });

    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(fileInput, file);

    expect(onImport).toHaveBeenCalledWith(['Turdus merula_Common Blackbird']);
    expect(await screen.findByText(KEY_IMPORT_SUMMARY)).toBeInTheDocument();
  });

  it('shows a rejected-rows toggle and reveals rows on click, without calling onImport for an all-rejected file', async () => {
    const { onImport } = renderField();
    const user = userEvent.setup();

    const csv = 'Common Name,Scientific Name\nGarbage,not a species\n';
    const file = new File([csv], 'life-list.csv', { type: 'text/csv' });

    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(fileInput, file);

    expect(onImport).not.toHaveBeenCalled();

    const toggle = await screen.findByText(KEY_SHOW_REJECTED_ROWS);
    await user.click(toggle);
    expect(screen.getByText(/not a species/i)).toBeInTheDocument();
  });

  it('filters the visible list via the search box once there are enough entries', async () => {
    const many = Array.from({ length: 9 }, (_, i) => `Species number${i}_Species ${i}`);
    renderField({ species: many });
    const user = userEvent.setup();

    const search = screen.getByLabelText(KEY_SEARCH_PLACEHOLDER);
    await user.type(search, 'Species 3');

    expect(screen.getByText('Species 3')).toBeInTheDocument();
    expect(screen.queryByText('Species 0')).not.toBeInTheDocument();
  });

  it('dismisses the import summary when Dismiss is clicked', async () => {
    renderField();
    const user = userEvent.setup();

    const csv = 'Common Name,Scientific Name\nCommon Blackbird,Turdus merula\n';
    const file = new File([csv], 'life-list.csv', { type: 'text/csv' });
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(fileInput, file);

    const dismissButton = await screen.findByText(KEY_DISMISS);
    await fireEvent.click(dismissButton);

    expect(screen.queryByText(KEY_DISMISS)).not.toBeInTheDocument();
  });
});
