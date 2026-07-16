import { describe, it, expect } from 'vitest';
import { parseLifeListCsv } from './lifeListCsv';

describe('parseLifeListCsv', () => {
  it('parses a well-formed eBird export', () => {
    const csv = [
      'Common Name,Scientific Name,Count,Location',
      'Common Blackbird,Turdus merula,1,"Backyard, Home"',
      'Great Tit,Parus major,3,Backyard',
    ].join('\n');

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toEqual(['Turdus merula_Common Blackbird', 'Parus major_Great Tit']);
    expect(result.rejected).toEqual([]);
  });

  it('is resilient to column reordering', () => {
    const csv = [
      'Location,Scientific Name,Count,Common Name',
      'Backyard,Turdus merula,1,Common Blackbird',
    ].join('\n');

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toEqual(['Turdus merula_Common Blackbird']);
  });

  it('rejects rows with an empty scientific name', () => {
    const csv = [
      'Common Name,Scientific Name',
      'Common Blackbird,Turdus merula',
      'Mystery Bird,',
    ].join('\n');

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toEqual(['Turdus merula_Common Blackbird']);
    expect(result.rejected).toHaveLength(1);
    expect(result.rejected[0].reason).toMatch(/empty scientific name/i);
    expect(result.rejected[0].rowNumber).toBe(3);
  });

  it('rejects rows that do not look like a binomial scientific name', () => {
    const csv = [
      'Common Name,Scientific Name',
      'Garbage Row,not a species name at all',
      'Common Blackbird,Turdus merula',
    ].join('\n');

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toEqual(['Turdus merula_Common Blackbird']);
    expect(result.rejected).toHaveLength(1);
    expect(result.rejected[0].rowNumber).toBe(2);
  });

  it('silently deduplicates repeated species (e.g. multiple sightings)', () => {
    const csv = [
      'Common Name,Scientific Name',
      'Common Blackbird,Turdus merula',
      'Common Blackbird,Turdus merula',
      'common blackbird,turdus merula',
    ].join('\n');

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toHaveLength(1);
    expect(result.rejected).toEqual([]);
  });

  it('handles quoted fields containing commas', () => {
    const csv = [
      'Common Name,Scientific Name,Location',
      'Common Blackbird,Turdus merula,"Springfield, IL, USA"',
    ].join('\n');

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toEqual(['Turdus merula_Common Blackbird']);
  });

  it('falls back to scientific-name-only entries when no Common Name column exists', () => {
    const csv = ['Scientific Name', 'Turdus merula'].join('\n');

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toEqual(['Turdus merula']);
  });

  it('returns empty results for an empty file', () => {
    const result = parseLifeListCsv('');
    expect(result.accepted).toEqual([]);
    expect(result.rejected).toEqual([]);
  });

  it('rejects the whole file when no Scientific Name column is found', () => {
    const csv = ['Common Name,Count', 'Common Blackbird,1'].join('\n');

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toEqual([]);
    expect(result.rejected).toHaveLength(1);
    expect(result.rejected[0].reason).toMatch(/No "Scientific Name" column/);
  });

  it('handles Windows-style CRLF line endings', () => {
    const csv = 'Common Name,Scientific Name\r\nCommon Blackbird,Turdus merula\r\n';

    const result = parseLifeListCsv(csv);

    expect(result.accepted).toEqual(['Turdus merula_Common Blackbird']);
  });
});
