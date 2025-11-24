import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useMutation } from '@tanstack/react-query';
import { apiClient } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import { useState } from 'react';
import { ArrowLeft, Upload, FileSpreadsheet } from 'lucide-react';
import { toast } from 'sonner';
import { Link } from '@tanstack/react-router';

export const Route = createFileRoute('/dashboard/users/bulk-upload')({
  component: BulkUploadPage,
});

function BulkUploadPage() {
  const navigate = useNavigate();
  const [file, setFile] = useState<File | null>(null);
  const [dragActive, setDragActive] = useState(false);

  const uploadMutation = useMutation({
    mutationFn: (file: File) => apiClient.bulkUploadUsers(file),
    onSuccess: (data) => {
      toast.success(
        `Successfully uploaded ${data.success} users. ${data.failed > 0 ? `${data.failed} failed.` : ''}`
      );
      if (data.errors && data.errors.length > 0) {
        console.error('Upload errors:', data.errors);
      }
      navigate({ to: '/dashboard/users' });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to upload users');
    },
  });

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
        setFile(droppedFile);
      } else {
        toast.error('Please upload an Excel file (.xlsx or .xls)');
      }
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const selectedFile = e.target.files[0];
      if (selectedFile.name.endsWith('.xlsx') || selectedFile.name.endsWith('.xls')) {
        setFile(selectedFile);
      } else {
        toast.error('Please upload an Excel file (.xlsx or .xls)');
      }
    }
  };

  const handleUpload = () => {
    if (!file) {
      toast.error('Please select a file');
      return;
    }

    uploadMutation.mutate(file);
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
            <CardTitle>Upload Excel File</CardTitle>
            <CardDescription>
              Upload a file with columns: Full Name, Rank, Battery, NRIC Last 4, DOB (DDMMYY)
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

            <div className="flex justify-end gap-2">
              <Button
                onClick={handleUpload}
                disabled={!file || uploadMutation.isPending}
              >
                <Upload className="mr-2 h-4 w-4" />
                {uploadMutation.isPending ? 'Uploading...' : 'Upload Users'}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}

