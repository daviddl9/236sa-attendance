import { createFileRoute, Link } from '@tanstack/react-router';
import { useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { toast } from 'sonner';
import { ArrowLeft, FileUp, Sparkles, AlertTriangle, CheckCircle2 } from 'lucide-react';

import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import { Input } from '../../../components/ui/input';
import { Label } from '../../../components/ui/label';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../../components/ui/table';
import { Badge } from '../../../components/ui/badge';
import {
  apiClient,
  type ImportPreviewResponse,
  type ImportPreviewedRow,
  type ImportMergeMode,
  type ImportCommitResponse,
} from '../../../lib/api-client';

const ACCEPTED_EXTENSIONS = ['.xlsx', '.xls', '.xlsm', '.docx', '.doc', '.pdf'] as const;
const ACCEPT_ATTR = ACCEPTED_EXTENSIONS.join(',');

function hasAcceptedExtension(name: string): boolean {
  const lower = name.toLowerCase();
  return ACCEPTED_EXTENSIONS.some((ext) => lower.endsWith(ext));
}

export const Route = createFileRoute('/dashboard/users/import-document')({
  component: ImportDocumentPage,
});

function ImportDocumentPage() {
  const [file, setFile] = useState<File | null>(null);
  const [dragActive, setDragActive] = useState(false);
  const [preview, setPreview] = useState<ImportPreviewResponse | null>(null);
  const [mergeMode, setMergeMode] = useState<ImportMergeMode>('fill_gaps');
  const [commitResult, setCommitResult] = useState<ImportCommitResponse | null>(null);

  const previewMutation = useMutation({
    mutationFn: (f: File) => apiClient.previewImportDocument(f),
    onSuccess: (data) => {
      setPreview(data);
      setCommitResult(null);
      toast.success(
        `Parsed ${data.rows.length} row${data.rows.length === 1 ? '' : 's'} from ${data.filename}`,
      );
    },
    onError: (err: Error) => {
      setPreview(null);
      toast.error(err.message || 'Failed to parse document');
    },
  });

  const commitMutation = useMutation({
    mutationFn: () => {
      if (!preview) throw new Error('No preview to commit');
      const rows = preview.rows
        .filter((r) => r.matchStatus !== 'invalid')
        .map((r) => ({
          fullName: r.fullName,
          rank: r.rank,
          battery: r.battery,
          nricLast5: r.nricLast5,
        }));
      return apiClient.commitImport({ importId: preview.importId, mergeMode, rows });
    },
    onSuccess: (data) => {
      setCommitResult(data);
      setPreview(null);
      toast.success('Import committed successfully');
    },
    onError: (err: Error) => {
      toast.error(err.message || 'Commit failed');
    },
  });

  const handleFile = (selected: File) => {
    if (!hasAcceptedExtension(selected.name)) {
      toast.error('Unsupported file type — upload .xlsx, .xls, .xlsm, .docx, .doc, or .pdf');
      return;
    }
    setFile(selected);
    setPreview(null);
    setCommitResult(null);
  };

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === 'dragenter' || e.type === 'dragover') setDragActive(true);
    else if (e.type === 'dragleave') setDragActive(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    const dropped = e.dataTransfer.files?.[0];
    if (dropped) handleFile(dropped);
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const selected = e.target.files?.[0];
    if (selected) handleFile(selected);
  };

  const handlePreview = () => {
    if (!file) return;
    previewMutation.mutate(file);
  };

  const committableCount = preview?.rows.filter((r) => r.matchStatus !== 'invalid').length ?? 0;

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
            <h1 className="text-3xl font-semibold tracking-tight">
              Import Personnel via Document
            </h1>
            <p className="text-muted-foreground">
              Upload an Excel, Word, or PDF roster — a Claude agent extracts the personnel rows for you to review before any database write.
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Sparkles className="h-4 w-4" />
              Document Upload
            </CardTitle>
            <CardDescription>
              Supported formats: .xlsx, .xls, .xlsm, .docx, .doc, .pdf — up to 10 MB. The document is sent to the Anthropic API for structured extraction; no rows are written to the user table until you confirm below.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div
              onDragEnter={handleDrag}
              onDragLeave={handleDrag}
              onDragOver={handleDrag}
              onDrop={handleDrop}
              className={`flex flex-col items-center justify-center rounded-lg border-2 border-dashed p-10 transition-colors ${
                dragActive ? 'border-primary bg-primary/5' : 'border-muted-foreground/30'
              }`}
            >
              <FileUp className="h-10 w-10 text-muted-foreground" />
              <p className="mt-3 text-sm">
                Drag & drop a document here, or
              </p>
              <label className="mt-2">
                <Input
                  type="file"
                  className="hidden"
                  accept={ACCEPT_ATTR}
                  onChange={handleFileChange}
                />
                <Button variant="outline" asChild>
                  <span>Choose file</span>
                </Button>
              </label>
              {file && (
                <p className="mt-3 text-sm text-muted-foreground">
                  Selected: <span className="font-medium">{file.name}</span> ({(file.size / 1024).toFixed(1)} KB)
                </p>
              )}
            </div>

            <div className="flex justify-end">
              <Button
                onClick={handlePreview}
                disabled={!file || previewMutation.isPending}
              >
                {previewMutation.isPending ? 'Parsing document…' : 'Parse with Claude agent'}
              </Button>
            </div>
          </CardContent>
        </Card>

        {preview && (
          <>
            <PreviewBlock preview={preview} />

            <Card>
              <CardHeader>
                <CardTitle>Confirm Import</CardTitle>
                <CardDescription>
                  {committableCount} row{committableCount === 1 ? '' : 's'} will be written.
                  Invalid rows are excluded automatically.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="space-y-3">
                  <p className="text-sm font-medium">Merge mode for existing users</p>
                  <div className="flex flex-col gap-3">
                    <label className="flex items-start gap-3 cursor-pointer">
                      <input
                        type="radio"
                        name="mergeMode"
                        value="fill_gaps"
                        checked={mergeMode === 'fill_gaps'}
                        onChange={() => setMergeMode('fill_gaps')}
                        className="mt-0.5"
                      />
                      <div>
                        <Label className="cursor-pointer font-medium">Fill gaps only</Label>
                        <p className="text-sm text-muted-foreground">
                          Only populate empty fields on existing users. Pre-filled values are kept.
                        </p>
                      </div>
                    </label>
                    <label className="flex items-start gap-3 cursor-pointer">
                      <input
                        type="radio"
                        name="mergeMode"
                        value="override"
                        checked={mergeMode === 'override'}
                        onChange={() => setMergeMode('override')}
                        className="mt-0.5"
                      />
                      <div>
                        <Label className="cursor-pointer font-medium">Override fields from document</Label>
                        <p className="text-sm text-muted-foreground">
                          Replace rank and battery on existing users with document values. NRIC Last 5 and password are never changed.
                        </p>
                      </div>
                    </label>
                  </div>
                </div>

                <div className="flex justify-end">
                  <Button
                    onClick={() => commitMutation.mutate()}
                    disabled={commitMutation.isPending || committableCount === 0}
                  >
                    {commitMutation.isPending
                      ? 'Committing…'
                      : `Commit ${committableCount} row${committableCount === 1 ? '' : 's'}`}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </>
        )}

        {commitResult && <CommitResultBlock result={commitResult} />}
      </div>
    </DashboardLayout>
  );
}

function PreviewBlock({ preview }: { preview: ImportPreviewResponse }) {
  const { rows, rowCounts, filename } = preview;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Preview — {filename}</CardTitle>
        <CardDescription>
          {rows.length} row{rows.length === 1 ? '' : 's'} parsed.{' '}
          <span className="font-medium text-emerald-600">{rowCounts.created}</span> new ·{' '}
          <span className="font-medium text-amber-600">{rowCounts.updated}</span> existing match ·{' '}
          <span className="font-medium text-rose-600">{rowCounts.skipped}</span> invalid.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Status</TableHead>
                <TableHead>Full Name</TableHead>
                <TableHead>Rank</TableHead>
                <TableHead>Battery</TableHead>
                <TableHead>NRIC Last 5</TableHead>
                <TableHead>Confidence</TableHead>
                <TableHead>Source / validation</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row, i) => (
                <PreviewRow key={i} row={row} />
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  );
}

function PreviewRow({ row }: { row: ImportPreviewedRow }) {
  return (
    <TableRow>
      <TableCell>
        <StatusBadge row={row} />
      </TableCell>
      <TableCell className="font-medium">{row.fullName || <span className="text-muted-foreground italic">(empty)</span>}</TableCell>
      <TableCell>{row.rank || '—'}</TableCell>
      <TableCell>{row.battery || '—'}</TableCell>
      <TableCell className="font-mono">{row.nricLast5 || '—'}</TableCell>
      <TableCell>{(row.confidence * 100).toFixed(0)}%</TableCell>
      <TableCell className="max-w-md">
        {row.validationMsg ? (
          <span className="text-rose-600">{row.validationMsg}</span>
        ) : (
          <span className="text-muted-foreground text-sm">{row.sourceSnippet}</span>
        )}
      </TableCell>
    </TableRow>
  );
}

function StatusBadge({ row }: { row: ImportPreviewedRow }) {
  switch (row.matchStatus) {
    case 'new':
      return (
        <Badge variant="default" className="gap-1">
          <CheckCircle2 className="h-3 w-3" />
          New
        </Badge>
      );
    case 'existing_match':
      return <Badge variant="secondary">Existing match</Badge>;
    case 'invalid':
      return (
        <Badge variant="destructive" className="gap-1">
          <AlertTriangle className="h-3 w-3" />
          Invalid
        </Badge>
      );
  }
}

function CommitResultBlock({ result }: { result: ImportCommitResponse }) {
  const { rowCounts } = result;
  return (
    <Card className="border-emerald-200 bg-emerald-50 dark:border-emerald-800 dark:bg-emerald-950/20">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-emerald-700 dark:text-emerald-400">
          <CheckCircle2 className="h-5 w-5" />
          Import complete
        </CardTitle>
        <CardDescription>
          <span className="font-medium text-emerald-600">{rowCounts.created}</span> created ·{' '}
          <span className="font-medium text-amber-600">{rowCounts.updated}</span> updated ·{' '}
          <span className="font-medium text-rose-600">{rowCounts.skipped}</span> skipped
        </CardDescription>
      </CardHeader>
    </Card>
  );
}
