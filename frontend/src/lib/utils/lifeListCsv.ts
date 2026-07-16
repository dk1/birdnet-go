/**
 * Parses an eBird life list CSV export into life-list entries.
 *
 * Only structural parseability is validated here (a non-empty, binomial-looking
 * scientific name) — not whether the species exists in BirdNET's own taxonomy or
 * eBird's current taxonomy. That matching happens on the backend, continuously,
 * at detection time (see internal/conf.Settings.IsOnLifeList); attempting to
 * validate species identity at import time would be misleading given known
 * taxonomy-version mismatches (splits/lumps) that can't be resolved client-side.
 */

export interface LifeListParseRejection {
  rowNumber: number; // 1-based, counting the header as row 1
  rawRow: string;
  reason: string;
}

export interface LifeListParseResult {
  accepted: string[]; // "ScientificName_CommonName" entries, deduplicated
  rejected: LifeListParseRejection[];
}

const SCIENTIFIC_NAME_HEADERS = ['scientific name', 'sci_name', 'scientific_name'];
const COMMON_NAME_HEADERS = ['common name', 'com_name', 'common_name'];

// Two space-separated words, letters (and hyphens, for hyphenated epithets)
// only, optionally followed by a subspecies epithet. Deliberately permissive:
// this only screens out obviously-garbage rows (blank cells, header-like
// repeats, free-text notes), not taxonomically invalid names.
// eslint-disable-next-line security/detect-unsafe-regex -- three space-separated quantified groups with no overlap or nesting; linear-time, not susceptible to catastrophic backtracking
const BINOMIAL_PATTERN = /^[A-Za-z][A-Za-z-]+ [a-z][a-z-]+( [a-z][a-z-]+)?$/;

/**
 * Splits a single CSV row into fields, honoring double-quoted fields that may
 * contain commas (eBird's Location column commonly does). Does not handle
 * escaped quotes inside quoted fields beyond the standard "" doubling.
 */
function splitCsvRow(row: string): string[] {
  const fields: string[] = [];
  let current = '';
  let inQuotes = false;

  for (let i = 0; i < row.length; i++) {
    // eslint-disable-next-line security/detect-object-injection -- i is a numeric loop counter bounded by row.length
    const char = row[i];

    if (inQuotes) {
      if (char === '"') {
        if (row[i + 1] === '"') {
          current += '"';
          i++;
        } else {
          inQuotes = false;
        }
      } else {
        current += char;
      }
      continue;
    }

    if (char === '"') {
      inQuotes = true;
    } else if (char === ',') {
      fields.push(current);
      current = '';
    } else {
      current += char;
    }
  }
  fields.push(current);

  return fields.map(f => f.trim());
}

function findColumn(header: string[], candidates: string[]): number {
  const normalized = header.map(h => h.toLowerCase().trim());
  for (const candidate of candidates) {
    const idx = normalized.indexOf(candidate);
    if (idx !== -1) return idx;
  }
  return -1;
}

export function parseLifeListCsv(csvText: string): LifeListParseResult {
  const lines = csvText.split(/\r\n|\r|\n/).filter(line => line.trim().length > 0);

  const accepted: string[] = [];
  const rejected: LifeListParseRejection[] = [];
  const seen = new Set<string>();

  if (lines.length === 0) {
    return { accepted, rejected };
  }

  const header = splitCsvRow(lines[0]);
  const sciCol = findColumn(header, SCIENTIFIC_NAME_HEADERS);
  const comCol = findColumn(header, COMMON_NAME_HEADERS);

  if (sciCol === -1) {
    rejected.push({
      rowNumber: 1,
      rawRow: lines[0],
      reason: 'No "Scientific Name" column found in header',
    });
    return { accepted, rejected };
  }

  for (let i = 1; i < lines.length; i++) {
    const rowNumber = i + 1;
    // eslint-disable-next-line security/detect-object-injection -- i is a numeric loop counter bounded by lines.length
    const rawRow = lines[i];
    const fields = splitCsvRow(rawRow);

    // eslint-disable-next-line security/detect-object-injection -- sciCol is a numeric column index resolved by findColumn, not user-controlled key access
    const scientificName = (fields[sciCol] ?? '').trim();
    // eslint-disable-next-line security/detect-object-injection -- comCol is a numeric column index resolved by findColumn, not user-controlled key access
    const commonName = comCol !== -1 ? (fields[comCol] ?? '').trim() : '';

    if (!scientificName) {
      rejected.push({ rowNumber, rawRow, reason: 'Empty scientific name' });
      continue;
    }
    if (!BINOMIAL_PATTERN.test(scientificName)) {
      rejected.push({
        rowNumber,
        rawRow,
        reason: `"${scientificName}" does not look like a scientific name`,
      });
      continue;
    }

    const entry = commonName ? `${scientificName}_${commonName}` : scientificName;
    const dedupeKey = entry.toLowerCase();
    if (seen.has(dedupeKey)) {
      continue; // silent dedupe, not a rejection
    }
    seen.add(dedupeKey);
    accepted.push(entry);
  }

  return { accepted, rejected };
}
