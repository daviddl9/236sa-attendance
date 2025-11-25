import { createFileRoute } from '@tanstack/react-router';
import { useMutation, useQuery } from '@tanstack/react-query';
import { apiClient } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '../../../components/ui/dialog';
import { Input } from '../../../components/ui/input';
import { Label } from '../../../components/ui/label';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import { useState, useRef, useEffect } from 'react';
import { ScanLine, CheckCircle2, XCircle } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '../../../lib/auth-context';
import { canAccessCommanderFeatures } from '../../../lib/user-utils';

export const Route = createFileRoute('/dashboard/attendance/scan')({
  component: ScanAttendancePage,
});

function ScanAttendancePage() {
  const { user } = useAuth();
  const [scanning, setScanning] = useState(false);
  const [scannedData, setScannedData] = useState<string | null>(null);
  const [scanResult, setScanResult] = useState<'success' | 'error' | null>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);

  const canViewSessions = canAccessCommanderFeatures(user);

  const { data: activeSessions } = useQuery({
    queryKey: ['sessions', 'active'],
    queryFn: () => apiClient.getActiveSessions(),
    enabled: canViewSessions,
  });

  const markMutation = useMutation({
    mutationFn: (qrData: string) => apiClient.markAttendance({ qrData }),
    onSuccess: () => {
      setScanResult('success');
      toast.success('Attendance marked successfully!');
      setTimeout(() => {
        setScanResult(null);
        setScannedData(null);
      }, 3000);
    },
    onError: (error: Error) => {
      setScanResult('error');
      toast.error(error.message || 'Failed to mark attendance');
      setTimeout(() => {
        setScanResult(null);
        setScannedData(null);
      }, 3000);
    },
  });

  const startScanning = async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { facingMode: 'environment' },
      });
      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        setScanning(true);
      }
    } catch {
      toast.error('Failed to access camera. Please grant camera permissions.');
    }
  };

  const stopScanning = () => {
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    }
    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }
    setScanning(false);
  };

  useEffect(() => {
    return () => {
      stopScanning();
    };
  }, []);

  const [manualQRDialogOpen, setManualQRDialogOpen] = useState(false);
  const [manualQRInput, setManualQRInput] = useState('');

  const handleManualQR = () => {
    if (manualQRInput.trim()) {
      setScannedData(manualQRInput.trim());
      markMutation.mutate(manualQRInput.trim());
      setManualQRInput('');
      setManualQRDialogOpen(false);
    }
  };

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Scan QR Code</h1>
          <p className="text-muted-foreground">Scan a QR code to mark your attendance</p>
        </div>

        {canViewSessions && activeSessions && activeSessions.length === 0 && (
          <Card>
            <CardContent className="pt-6">
              <p className="text-center text-muted-foreground">
                No active sessions available. Please wait for a session to be created.
              </p>
            </CardContent>
          </Card>
        )}

        <Card>
          <CardHeader>
            <CardTitle>QR Code Scanner</CardTitle>
            <CardDescription>
              Point your camera at the QR code to mark attendance
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="relative aspect-square max-w-md mx-auto border rounded-lg overflow-hidden bg-black">
              {scanning ? (
                <video
                  ref={videoRef}
                  autoPlay
                  playsInline
                  className="w-full h-full object-cover"
                />
              ) : (
                <div className="w-full h-full flex items-center justify-center">
                  <ScanLine className="h-24 w-24 text-muted-foreground" />
                </div>
              )}
              {scanResult === 'success' && (
                <div className="absolute inset-0 bg-green-500/20 flex items-center justify-center">
                  <CheckCircle2 className="h-24 w-24 text-green-500" />
                </div>
              )}
              {scanResult === 'error' && (
                <div className="absolute inset-0 bg-red-500/20 flex items-center justify-center">
                  <XCircle className="h-24 w-24 text-red-500" />
                </div>
              )}
            </div>

            <div className="flex gap-2 justify-center">
              {!scanning ? (
                <>
                  <Button onClick={startScanning}>
                    <ScanLine className="mr-2 h-4 w-4" />
                    Start Scanning
                  </Button>
                  <Dialog open={manualQRDialogOpen} onOpenChange={setManualQRDialogOpen}>
                    <DialogTrigger asChild>
                      <Button variant="outline">
                        Enter Manually
                      </Button>
                    </DialogTrigger>
                    <DialogContent>
                      <DialogHeader>
                        <DialogTitle>Enter QR Code Data</DialogTitle>
                        <DialogDescription>
                          Manually enter the QR code data to mark attendance
                        </DialogDescription>
                      </DialogHeader>
                      <div className="space-y-4 py-4">
                        <div className="grid gap-2">
                          <Label htmlFor="qr-data">QR Code Data</Label>
                          <Input
                            id="qr-data"
                            value={manualQRInput}
                            onChange={(e) => setManualQRInput(e.target.value)}
                            placeholder="Enter QR code data..."
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') {
                                handleManualQR();
                              }
                            }}
                          />
                        </div>
                        <div className="flex justify-end gap-2">
                          <Button
                            variant="outline"
                            onClick={() => {
                              setManualQRDialogOpen(false);
                              setManualQRInput('');
                            }}
                          >
                            Cancel
                          </Button>
                          <Button onClick={handleManualQR} disabled={!manualQRInput.trim()}>
                            Submit
                          </Button>
                        </div>
                      </div>
                    </DialogContent>
                  </Dialog>
                </>
              ) : (
                <Button onClick={stopScanning} variant="destructive">
                  Stop Scanning
                </Button>
              )}
            </div>

            {scannedData && (
              <div className="text-center">
                <p className="text-sm text-muted-foreground">Scanned:</p>
                <p className="text-xs font-mono break-all">{scannedData}</p>
              </div>
            )}

            {markMutation.isPending && (
              <div className="text-center text-muted-foreground">
                Processing...
              </div>
            )}
          </CardContent>
        </Card>

        {canViewSessions && activeSessions && activeSessions.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Active Sessions</CardTitle>
              <CardDescription>Currently active attendance sessions</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                {activeSessions.map((session) => (
                  <div
                    key={session.id}
                    className="flex items-center justify-between p-3 border rounded"
                  >
                    <div>
                      <p className="font-medium">{session.name}</p>
                      <p className="text-sm text-muted-foreground">
                        {new Date(session.startTime).toLocaleString()}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </DashboardLayout>
  );
}

