import * as XLSX from 'xlsx';

export interface ParsedRow {
  fullName: string;
  rank: string;
  battery: string;
  nricLast5: string;
}

export interface ParseResult {
  rows: ParsedRow[];
  skipped: number;
  sheetName: string;
  totalRawRows: number;
}

interface ColumnMapping {
  fullName: number;
  rank: number;
  battery: number;
  nricLast5: number;
  callupCol: number; // -1 if not found
  headerRow: number;
}

/**
 * Parse a callup Excel file, auto-detecting the data sheet and column layout.
 * Handles both simple 4-column format and real SAF callup files with 16+ columns.
 */
export function parseExcelFile(file: File): Promise<ParseResult> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = (e) => {
      try {
        const data = new Uint8Array(e.target?.result as ArrayBuffer);
        const workbook = XLSX.read(data, { type: 'array' });

        // Find the sheet with the most rows
        const sheetName = findDataSheet(workbook);
        if (!sheetName) {
          reject(new Error('No valid data sheet found in Excel file'));
          return;
        }

        const sheet = workbook.Sheets[sheetName];
        const rawRows: string[][] = XLSX.utils.sheet_to_json(sheet, {
          header: 1,
          defval: '',
          raw: false,
        });

        if (rawRows.length < 2) {
          reject(new Error('Sheet must have at least a header row and one data row'));
          return;
        }

        // Detect column mapping from headers
        const colMap = detectColumns(rawRows);

        let nameIdx: number, rankIdx: number, batteryIdx: number, nricIdx: number;
        let callupIdx: number, dataStartRow: number;

        if (colMap) {
          nameIdx = colMap.fullName;
          rankIdx = colMap.rank;
          batteryIdx = colMap.battery;
          nricIdx = colMap.nricLast5;
          callupIdx = colMap.callupCol;
          dataStartRow = colMap.headerRow + 1;
        } else {
          // Fallback: assume first row is header, columns [0,1,2,3]
          nameIdx = 0;
          rankIdx = 1;
          batteryIdx = 2;
          nricIdx = 3;
          callupIdx = -1;
          dataStartRow = 1;
        }

        const dataRows = rawRows.slice(dataStartRow);
        const rows: ParsedRow[] = [];
        let skipped = 0;

        for (const row of dataRows) {
          const fullName = cellValue(row, nameIdx);
          const rank = cellValue(row, rankIdx);
          const battery = cellValue(row, batteryIdx);
          const nricLast5 = cellValue(row, nricIdx);

          // Skip empty rows
          if (!fullName && !rank && !battery && !nricLast5) {
            continue;
          }

          // Filter by callup status if column detected
          if (callupIdx >= 0) {
            const callupStatus = cellValue(row, callupIdx).toUpperCase();
            if (callupStatus.includes('NO CALL UP') || callupStatus === '') {
              skipped++;
              continue;
            }
          }

          rows.push({ fullName, rank, battery, nricLast5 });
        }

        resolve({
          rows,
          skipped,
          sheetName,
          totalRawRows: rawRows.length,
        });
      } catch (err) {
        reject(new Error(`Failed to parse Excel file: ${err instanceof Error ? err.message : err}`));
      }
    };
    reader.onerror = () => reject(new Error('Failed to read file'));
    reader.readAsArrayBuffer(file);
  });
}

function findDataSheet(workbook: XLSX.WorkBook): string | null {
  const sheets = workbook.SheetNames;
  if (sheets.length === 0) return null;

  let bestSheet = sheets[0];
  let bestRows = 0;

  for (const name of sheets) {
    const sheet = workbook.Sheets[name];
    const ref = sheet['!ref'];
    if (!ref) continue;

    const range = XLSX.utils.decode_range(ref);
    const rowCount = range.e.r + 1;
    if (rowCount > bestRows) {
      bestRows = rowCount;
      bestSheet = name;
    }
  }

  return bestSheet;
}

function detectColumns(rows: string[][]): ColumnMapping | null {
  const maxScan = Math.min(5, rows.length);

  for (let i = 0; i < maxScan; i++) {
    const row = rows[i];
    const mapping: ColumnMapping = {
      fullName: -1,
      rank: -1,
      battery: -1,
      nricLast5: -1,
      callupCol: -1,
      headerRow: i,
    };

    for (let j = 0; j < row.length; j++) {
      const lower = String(row[j] ?? '').trim().toLowerCase();
      if (!lower) continue;

      if (mapping.fullName === -1 && (lower === 'full name' || lower === 'name' || lower === 'fullname')) {
        mapping.fullName = j;
      } else if (mapping.rank === -1 && lower === 'rank') {
        mapping.rank = j;
      } else if (mapping.battery === -1 && (lower === 'battery' || lower === 'bty')) {
        mapping.battery = j;
      } else if (mapping.nricLast5 === -1 && lower.includes('nric')) {
        mapping.nricLast5 = j;
      } else if (mapping.callupCol === -1 && (lower.includes('call up') || lower.includes('callup'))) {
        mapping.callupCol = j;
      }
    }

    if (mapping.fullName >= 0 && mapping.rank >= 0 && mapping.battery >= 0 && mapping.nricLast5 >= 0) {
      return mapping;
    }
  }

  return null;
}

function cellValue(row: string[], idx: number): string {
  if (idx < 0 || idx >= row.length) return '';
  return String(row[idx] ?? '').trim();
}
