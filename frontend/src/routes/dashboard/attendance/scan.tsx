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
import jsQR from 'jsqr';
import { ScanLine, CheckCircle2, XCircle, SwitchCamera } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '../../../lib/auth-context';
import { canAccessCommanderFeatures } from '../../../lib/user-utils';

export const Route = createFileRoute('/dashboard/attendance/scan')({
  component: ScanAttendancePage,
});

// Session QR codes encode a URL like https://host/qr/<sessionId>:<secret>.
// The mark-attendance endpoint expects "<sessionId>:<secret>:<timestamp>".
// Accept either the full URL or a bare token and normalize to that shape.
function toQrData(raw: string): string | null {
  const text = raw.trim();
  const token = text.includes('/qr/') ? text.split('/qr/')[1] : text;
  const cleaned = token.split(/[?#]/)[0];
  const parts = cleaned.split(':');
  if (parts.length < 2 || !parts[0] || !parts[1]) return null;
  return `${parts[0]}:${parts[1]}:${Math.floor(Date.now() / 1000)}`;
}

// Acquire a camera stream that works across devices. Without a deviceId we
// prefer the rear camera (best for scanning on phones/tablets) but only as a
// preference, so laptops/desktops with a front camera still get a stream.
// A specific request that a device can't satisfy falls back to any camera.
async function getCameraStream(deviceId?: string): Promise<MediaStream> {
  const constraints: MediaStreamConstraints = deviceId
    ? { video: { deviceId: { exact: deviceId } } }
    : { video: { facingMode: { ideal: 'environment' } } };
  try {
    return await navigator.mediaDevices.getUserMedia(constraints);
  } catch (err) {
    if (err instanceof DOMException && err.name === 'OverconstrainedError') {
      return await navigator.mediaDevices.getUserMedia({ video: true });
    }
    throw err;
  }
}

// Map the various getUserMedia failures to guidance the user can act on.
function cameraErrorMessage(err: unknown): string {
  const name = err instanceof DOMException ? err.name : '';
  switch (name) {
    case 'NotAllowedError':
    case 'SecurityError':
      return 'Camera permission denied. Allow camera access in your browser, then try again.';
    case 'NotFoundError':
    case 'OverconstrainedError':
      return 'No camera found on this device. Use "Enter Manually" instead.';
    case 'NotReadableError':
      return 'The camera is in use by another app. Close it and try again.';
    default:
      return 'Failed to access camera. Please grant camera permissions and try again.';
  }
}

function ScanAttendancePage() {
  const { user } = useAuth();
  const [scanning, setScanning] = useState(false);
  const [scannedData, setScannedData] = useState<string | null>(null);
  const [scanResult, setScanResult] = useState<'success' | 'error' | null>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const [cameras, setCameras] = useState<MediaDeviceInfo[]>([]);

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
    if (!navigator.mediaDevices?.getUserMedia) {
      toast.error('Camera is not supported in this browser. Use "Enter Manually" instead.');
      return;
    }
    try {
      const stream = await getCameraStream();
      streamRef.current = stream;
      // Flip to scanning so the <video> mounts; the effect below wires the
      // stream to it and starts the decode loop once it's in the DOM.
      setScanning(true);
      // Device labels are only exposed after permission is granted — list the
      // cameras now so multi-camera devices can switch between them.
      try {
        const all = await navigator.mediaDevices.enumerateDevices();
        setCameras(all.filter((d) => d.kind === 'videoinput'));
      } catch {
        // Enumeration is best-effort; scanning still works without it.
      }
    } catch (err) {
      toast.error(cameraErrorMessage(err));
    }
  };

  const switchCamera = async () => {
    if (cameras.length < 2) return;
    const currentId = streamRef.current?.getVideoTracks()[0]?.getSettings().deviceId;
    const idx = cameras.findIndex((c) => c.deviceId === currentId);
    const next = cameras[(idx + 1) % cameras.length];
    try {
      const stream = await getCameraStream(next.deviceId);
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        videoRef.current.play().catch(() => {});
      }
    } catch (err) {
      toast.error(cameraErrorMessage(err));
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

  const handleScanResult = (raw: string) => {
    const qrData = toQrData(raw);
    if (!qrData) {
      toast.error('Unrecognized QR code. Please try again.');
      return;
    }
    setScannedData(raw);
    stopScanning();
    markMutation.mutate(qrData);
  };
  // Keep the latest handler in a ref so the decode loop never goes stale and
  // the camera effect doesn't re-subscribe on every render.
  const handleScanResultRef = useRef(handleScanResult);
  useEffect(() => {
    handleScanResultRef.current = handleScanResult;
  });

  // Attach the camera stream and run the QR decode loop while scanning.
  useEffect(() => {
    if (!scanning) return;
    const video = videoRef.current;
    const stream = streamRef.current;
    if (!video || !stream) return;

    video.srcObject = stream;
    video.play().catch(() => {});

    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    let raf = 0;
    let stopped = false;

    const tick = () => {
      if (stopped) return;
      if (ctx && video.readyState === video.HAVE_ENOUGH_DATA && video.videoWidth > 0) {
        canvas.width = video.videoWidth;
        canvas.height = video.videoHeight;
        ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
        const image = ctx.getImageData(0, 0, canvas.width, canvas.height);
        const code = jsQR(image.data, image.width, image.height, {
          inversionAttempts: 'dontInvert',
        });
        if (code?.data) {
          stopped = true;
          handleScanResultRef.current(code.data);
          return;
        }
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);

    return () => {
      stopped = true;
      cancelAnimationFrame(raf);
    };
  }, [scanning]);

  useEffect(() => {
    return () => {
      stopScanning();
    };
  }, []);

  const [manualQRDialogOpen, setManualQRDialogOpen] = useState(false);
  const [manualQRInput, setManualQRInput] = useState('');

  const handleManualQR = () => {
    const trimmed = manualQRInput.trim();
    const qrData = toQrData(trimmed);
    if (!qrData) {
      toast.error('Unrecognized QR code.');
      return;
    }
    setScannedData(trimmed);
    markMutation.mutate(qrData);
    setManualQRInput('');
    setManualQRDialogOpen(false);
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
                  muted
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
                <>
                  {cameras.length > 1 && (
                    <Button onClick={switchCamera} variant="outline">
                      <SwitchCamera className="mr-2 h-4 w-4" />
                      Switch Camera
                    </Button>
                  )}
                  <Button onClick={stopScanning} variant="destructive">
                    Stop Scanning
                  </Button>
                </>
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

