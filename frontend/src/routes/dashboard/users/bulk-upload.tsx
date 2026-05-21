import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient, type BulkUploadRow } from '../../../lib/api-client';
import { parseExcelFile, type ParsedRow } from '../../../lib/parse-excel';
import { Input } from '../../../components/ui/input';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '../../../components/ui/dialog';
import { useState, useMemo } from 'react';
import { ArrowLeft, Upload, FileSpreadsheet, HelpCircle, CheckCircle2, AlertTriangle, Search, Columns3 } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from '../../../components/ui/dropdown-menu';
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '../../../components/ui/select';
import { toast } from 'sonner';
import { Link } from '@tanstack/react-router';
import { isValidNricLast5, normalizeNricLast5, NRIC_LAST5_FIELD_MESSAGE } from '../../../lib/nric-password';

const sampleData = [
  { fullName: 'John Doe', rank: 'PTE', battery: 'HQ', nricLast5: '1234A' },
  { fullName: 'Jane Smith', rank: 'CPL', battery: 'Alpha', nricLast5: '5678B' },
  { fullName: 'Bob Wilson', rank: 'LCP', battery: 'Bravo', nricLast5: '9012C' },
];

/** All displayable column keys: 4 required + extras */
const REQUIRED_COLUMNS = ['Full Name', 'Rank', 'Battery', 'NRIC Last 5'] as const;

function getExtraHeaders(headers: string[]): string[] {
  return headers.filter((h) => {
    const hLower = h.toLowerCase();
    return !(
      hLower === 'full name' || hLower === 'name' || hLower === 'fullname' ||
      hLower === 'rank' ||
      hLower === 'battery' || hLower === 'bty' ||
      hLower.includes('nric')
    );
  });
}

function getColumnValue(row: ParsedRow, col: string): string {
  switch (col) {
    case 'Full Name': return row.fullName;
    case 'Rank': return row.rank;
    case 'Battery': return row.battery;
    case 'NRIC Last 5': return row.nricLast5;
    default: return row.extras[col] ?? '';
  }
}

const MAX_FILTER_UNIQUE = 30;

function PreviewToolbar({
  parsedRows,
  headers,
  search,
  setSearch,
  visibleColumns,
  setVisibleColumns,
  columnFilters,
  setColumnFilters,
}: {
  parsedRows: ParsedRow[];
  headers: string[];
  search: string;
  setSearch: (v: string) => void;
  visibleColumns: Set<string>;
  setVisibleColumns: (v: Set<string>) => void;
  columnFilters: Record<string, string>;
  setColumnFilters: (v: Record<string, string>) => void;
}) {
  const extraHeaders = useMemo(() => getExtraHeaders(headers), [headers]);
  const allColumns = useMemo(() => [...REQUIRED_COLUMNS, ...extraHeaders], [extraHeaders]);

  // Compute unique values per column (only for columns with ≤ MAX_FILTER_UNIQUE unique values)
  const filterableColumns = useMemo(() => {
    const result: { col: string; values: string[] }[] = [];
    for (const col of allColumns) {
      const uniqueSet = new Set<string>();
      let tooMany = false;
      for (const row of parsedRows) {
        const v = getColumnValue(row, col);
        if (v) uniqueSet.add(v);
        if (uniqueSet.size > MAX_FILTER_UNIQUE) { tooMany = true; break; }
      }
      if (!tooMany && uniqueSet.size > 1) {
        result.push({ col, values: [...uniqueSet].sort() });
      }
    }
    return result;
  }, [parsedRows, allColumns]);

  // Filter rows
  const filtered = useMemo(() => {
    const term = search.toLowerCase();
    return parsedRows.filter((row) => {
      // Text search
      if (term) {
        const matchesText =
          row.fullName.toLowerCase().includes(term) ||
          row.rank.toLowerCase().includes(term) ||
          row.battery.toLowerCase().includes(term) ||
          row.nricLast5.toLowerCase().includes(term) ||
          Object.values(row.extras).some((v) => v.toLowerCase().includes(term));
        if (!matchesText) return false;
      }
      // Column filters (AND)
      for (const [col, value] of Object.entries(columnFilters)) {
        if (!value) continue;
        if (getColumnValue(row, col) !== value) return false;
      }
      return true;
    });
  }, [parsedRows, search, columnFilters]);

  const visibleExtraHeaders = extraHeaders.filter((h) => visibleColumns.has(h));
  const activeFilterCount = Object.values(columnFilters).filter(Boolean).length;

  return (
    <>
      {/* Toolbar row: search + columns dropdown */}
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search across all columns..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-8"
          />
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm">
              <Columns3 className="h-4 w-4 mr-1" />
              Columns
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48 max-h-72 overflow-y-auto">
            <DropdownMenuLabel>Toggle columns</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {allColumns.map((col) => (
              <DropdownMenuCheckboxItem
                key={col}
                checked={visibleColumns.has(col)}
                onCheckedChange={(checked) => {
                  const next = new Set(visibleColumns);
                  if (checked) next.add(col); else next.delete(col);
                  setVisibleColumns(next);
                  // Clear filter for hidden column
                  if (!checked && columnFilters[col]) {
                    const nextFilters = { ...columnFilters };
                    delete nextFilters[col];
                    setColumnFilters(nextFilters);
                  }
                }}
              >
                {col}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Per-column value filters */}
      {filterableColumns.length > 0 && (
        <div className="flex items-center gap-2 flex-wrap">
          {filterableColumns
            .filter(({ col }) => visibleColumns.has(col))
            .map(({ col, values }) => (
              <Select
                key={col}
                value={columnFilters[col] || '__all__'}
                onValueChange={(v) =>
                  setColumnFilters({ ...columnFilters, [col]: v === '__all__' ? '' : v })
                }
              >
                <SelectTrigger size="sm" className="h-7 text-xs gap-1">
                  <SelectValue placeholder={col} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all__">{col}: All</SelectItem>
                  {values.map((v) => (
                    <SelectItem key={v} value={v}>{v}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ))}
          {activeFilterCount > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 text-xs"
              onClick={() => setColumnFilters({})}
            >
              Clear filters
            </Button>
          )}
        </div>
      )}

      {/* Table */}
      <div className="max-h-64 overflow-auto rounded-md border">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-muted">
            <tr>
              <th className="p-2 text-left font-medium w-8">#</th>
              {visibleColumns.has('Full Name') && <th className="p-2 text-left font-medium">Full Name</th>}
              {visibleColumns.has('Rank') && <th className="p-2 text-left font-medium">Rank</th>}
              {visibleColumns.has('Battery') && <th className="p-2 text-left font-medium">Battery</th>}
              {visibleColumns.has('NRIC Last 5') && <th className="p-2 text-left font-medium">NRIC Last 5</th>}
              {visibleExtraHeaders.map((h) => (
                <th key={h} className="p-2 text-left font-medium whitespace-nowrap">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {filtered.slice(0, 50).map((row, i) => (
              <tr key={i} className="border-t">
                <td className="p-2 text-muted-foreground">{i + 1}</td>
                {visibleColumns.has('Full Name') && <td className="p-2">{row.fullName}</td>}
                {visibleColumns.has('Rank') && <td className="p-2">{row.rank}</td>}
                {visibleColumns.has('Battery') && <td className="p-2">{row.battery}</td>}
                {visibleColumns.has('NRIC Last 5') && <td className="p-2 font-mono">{row.nricLast5}</td>}
                {visibleExtraHeaders.map((h) => (
                  <td key={h} className="p-2 whitespace-nowrap">{row.extras[h] ?? ''}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        {filtered.length > 50 && (
          <div className="p-2 text-center text-xs text-muted-foreground border-t">
            Showing first 50 of {filtered.length} rows
          </div>
        )}
        {filtered.length === 0 && (
          <div className="p-4 text-center text-sm text-muted-foreground">
            No rows match your filters
          </div>
        )}
      </div>
      {(search || activeFilterCount > 0) && (
        <p className="text-xs text-muted-foreground">
          {filtered.length} of {parsedRows.length} rows match
        </p>
      )}
    </>
  );
}

export const Route = createFileRoute('/dashboard/users/bulk-upload')({
  component: BulkUploadPage,
});

function BulkUploadPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [dragActive, setDragActive] = useState(false);
  const [parsedRows, setParsedRows] = useState<ParsedRow[] | null>(null);
  const [headers, setHeaders] = useState<string[]>([]);
  const [parseInfo, setParseInfo] = useState<{ skipped: number; sheetName: string } | null>(null);
  const [parseError, setParseError] = useState<string | null>(null);
  const [isParsing, setIsParsing] = useState(false);
  const [search, setSearch] = useState('');
  const [visibleColumns, setVisibleColumns] = useState<Set<string>>(new Set());
  const [columnFilters, setColumnFilters] = useState<Record<string, string>>({});

  const uploadMutation = useMutation({
    mutationFn: (users: BulkUploadRow[]) => apiClient.bulkCreateUsers(users),
    onSuccess: (data) => {
      const parts = [`Successfully created ${data.success} users.`];
      if (data.failed > 0) parts.push(`${data.failed} failed.`);
      if (data.skipped && data.skipped > 0) parts.push(`${data.skipped} skipped.`);
      toast.success(parts.join(' '));
      if (data.errors && data.errors.length > 0) {
        console.error('Upload errors:', data.errors);
      }
      queryClient.invalidateQueries({ queryKey: ['users'] });
      navigate({ to: '/dashboard/users' });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to upload users');
    },
  });

  const handleFile = async (selectedFile: File) => {
    setFile(selectedFile);
    setParsedRows(null);
    setHeaders([]);
    setParseInfo(null);
    setParseError(null);
    setIsParsing(true);
    setSearch('');
    setColumnFilters({});

    try {
      const result = await parseExcelFile(selectedFile);
      setParsedRows(result.rows);
      setHeaders(result.headers);
      setParseInfo({ skipped: result.skipped, sheetName: result.sheetName });
      // All columns visible by default
      setVisibleColumns(new Set(['Full Name', 'Rank', 'Battery', 'NRIC Last 5', ...result.headers.filter((h) => {
        const hLower = h.toLowerCase();
        return !(
          hLower === 'full name' || hLower === 'name' || hLower === 'fullname' ||
          hLower === 'rank' ||
          hLower === 'battery' || hLower === 'bty' ||
          hLower.includes('nric')
        );
      })]));

      if (result.rows.length === 0) {
        setParseError('No valid rows found in the file. Check the column headers.');
      }
    } catch (err) {
      setParseError(err instanceof Error ? err.message : 'Failed to parse file');
    } finally {
      setIsParsing(false);
    }
  };

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true);
    } else if (e.type === 'dragleave') {
      setDragActive(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);

    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      const droppedFile = e.dataTransfer.files[0];
      if (droppedFile.name.endsWith('.xlsx') || droppedFile.name.endsWith('.xls')) {
        handleFile(droppedFile);
      } else {
        toast.error('Please upload an Excel file (.xlsx or .xls)');
      }
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const selectedFile = e.target.files[0];
      if (selectedFile.name.endsWith('.xlsx') || selectedFile.name.endsWith('.xls')) {
        handleFile(selectedFile);
      } else {
        toast.error('Please upload an Excel file (.xlsx or .xls)');
      }
    }
  };

  const handleUpload = () => {
    if (!parsedRows || parsedRows.length === 0) {
      toast.error('No valid rows to upload');
      return;
    }
    const invalidRows = parsedRows
      .map((row, index) => ({ row, rowNumber: index + 1 }))
      .filter(({ row }) => !isValidNricLast5(row.nricLast5));

    if (invalidRows.length > 0) {
      const rows = invalidRows.map(({ rowNumber }) => rowNumber).join(', ');
      toast.error(`${NRIC_LAST5_FIELD_MESSAGE}. Check row${invalidRows.length === 1 ? '' : 's'} ${rows}.`);
      return;
    }

    const payload = parsedRows.map(({ fullName, rank, battery, nricLast5, extras }) => ({
      fullName, rank, battery, nricLast5: normalizeNricLast5(nricLast5), extras,
    }));
    uploadMutation.mutate(payload);
  };

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center gap-4">
          <Link to="/dashboard/users">
            <Button variant="ghost" size="icon">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Bulk Upload Users</h1>
            <p className="text-muted-foreground">Upload users from an Excel file</p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>Upload Excel File</CardTitle>
              <Dialog>
                <DialogTrigger asChild>
                  <Button variant="outline" size="sm">
                    <HelpCircle className="h-4 w-4 mr-1" />
                    View Sample Format
                  </Button>
                </DialogTrigger>
                <DialogContent className="sm:max-w-xl">
                  <DialogHeader>
                    <DialogTitle>Sample Excel Format</DialogTitle>
                    <DialogDescription>
                      Your Excel file should have columns for Full Name, Rank, Battery, and NRIC Last 5.
                      Column order doesn't matter — headers are auto-detected.
                      SAF callup files are supported directly.
                    </DialogDescription>
                  </DialogHeader>
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm border-collapse">
                      <thead>
                        <tr className="border-b bg-muted/50">
                          <th className="p-2 text-left font-medium">Full Name</th>
                          <th className="p-2 text-left font-medium">Rank</th>
                          <th className="p-2 text-left font-medium">Battery</th>
                          <th className="p-2 text-left font-medium">NRIC Last 5</th>
                        </tr>
                      </thead>
                      <tbody>
                        {sampleData.map((row, index) => (
                          <tr key={index} className="border-b">
                            <td className="p-2">{row.fullName}</td>
                            <td className="p-2">{row.rank}</td>
                            <td className="p-2">{row.battery}</td>
                            <td className="p-2 font-mono">{row.nricLast5}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <div className="text-xs text-muted-foreground space-y-1">
                    <p><strong>Valid Ranks:</strong> REC, PTE, LCP, CPL, CFC, 3SG, 2SG, 1SG, SSG, MSG, 3WO, 2WO, 1WO, MWO, SWO, CWO, 2LT, LTA, CPT, MAJ, LTC, SLTC, COL, BG, MG, LG</p>
                    <p><strong>Valid Batteries:</strong> HQ, Alpha, Bravo</p>
                    <p><strong>NRIC Last 5:</strong> Four digits followed by a letter (e.g., 4567A). This will be the user's password.</p>
                  </div>
                </DialogContent>
              </Dialog>
            </div>
            <CardDescription>
              Upload a callup Excel file or a simple spreadsheet with Full Name, Rank, Battery, NRIC Last 5 columns
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div
              onDragEnter={handleDrag}
              onDragLeave={handleDrag}
              onDragOver={handleDrag}
              onDrop={handleDrop}
              className={`border-2 border-dashed rounded-lg p-8 text-center transition-colors ${
                dragActive
                  ? 'border-primary bg-primary/5'
                  : 'border-muted-foreground/25 hover:border-muted-foreground/50'
              }`}
            >
              <FileSpreadsheet className="mx-auto h-12 w-12 text-muted-foreground mb-4" />
              <div className="space-y-2">
                <p className="text-sm text-muted-foreground">
                  Drag and drop your Excel file here, or click to browse
                </p>
                <input
                  type="file"
                  accept=".xlsx,.xls"
                  onChange={handleFileChange}
                  className="hidden"
                  id="file-upload"
                />
                <label htmlFor="file-upload">
                  <Button variant="outline" asChild>
                    <span>Select File</span>
                  </Button>
                </label>
              </div>
              {file && (
                <div className="mt-4">
                  <p className="text-sm font-medium">Selected: {file.name}</p>
                  <p className="text-xs text-muted-foreground">
                    Size: {(file.size / 1024).toFixed(2)} KB
                  </p>
                </div>
              )}
            </div>

            {isParsing && (
              <div className="text-sm text-muted-foreground">Parsing file...</div>
            )}

            {parseError && (
              <div className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
                <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
                <span>{parseError}</span>
              </div>
            )}

            {parsedRows && parsedRows.length > 0 && parseInfo && (
              <div className="space-y-3">
                <div className="flex items-start gap-2 rounded-md border border-green-500/50 bg-green-500/10 p-3 text-sm">
                  <CheckCircle2 className="h-4 w-4 mt-0.5 shrink-0 text-green-600" />
                  <div>
                    <span className="font-medium">{parsedRows.length} personnel</span> parsed from sheet "{parseInfo.sheetName}"
                    {parseInfo.skipped > 0 && (
                      <span className="text-muted-foreground"> ({parseInfo.skipped} skipped — not called up)</span>
                    )}
                  </div>
                </div>

                <PreviewToolbar
                  parsedRows={parsedRows}
                  headers={headers}
                  search={search}
                  setSearch={setSearch}
                  visibleColumns={visibleColumns}
                  setVisibleColumns={setVisibleColumns}
                  columnFilters={columnFilters}
                  setColumnFilters={setColumnFilters}
                />
              </div>
            )}

            <div className="flex justify-end gap-2">
              <Button
                onClick={handleUpload}
                disabled={!parsedRows || parsedRows.length === 0 || uploadMutation.isPending}
              >
                <Upload className="mr-2 h-4 w-4" />
                {uploadMutation.isPending
                  ? 'Creating Users...'
                  : parsedRows && parsedRows.length > 0
                    ? `Upload ${parsedRows.length} Users`
                    : 'Upload Users'}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}
