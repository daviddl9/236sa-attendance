import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import { Input } from '../../../components/ui/input';
import { Label } from '../../../components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../../components/ui/select';
import { Switch } from '../../../components/ui/switch';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../../components/ui/card';
import { useState } from 'react';
import { ArrowLeft, Save } from 'lucide-react';
import { toast } from 'sonner';
import { Link } from '@tanstack/react-router';

export const Route = createFileRoute('/dashboard/sessions/create')({
  component: CreateSessionPage,
});

function CreateSessionPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [name, setName] = useState('');
  const [scope, setScope] = useState('unit_wide');
  const [batteries, setBatteries] = useState<string[]>([]);

  const createMutation = useMutation({
    mutationFn: (data: {
      name: string;
      scope: string;
      batteries?: string[];
    }) => apiClient.createSession(data),
    onSuccess: (data) => {
      toast.success('Session created successfully');
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      navigate({
        to: '/dashboard/sessions/$sessionId',
        params: { sessionId: data.id },
      });
    },
    onError: (error: Error) => {
      toast.error(error.message || 'Failed to create session');
    },
  });

  const handleBatteryChange = (battery: string, checked: boolean) => {
    if (checked) {
      setBatteries([...batteries, battery]);
    } else {
      setBatteries(batteries.filter((b) => b !== battery));
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    if (!name) {
      toast.error('Please fill in all required fields');
      return;
    }

    if (scope === 'battery_specific' && batteries.length === 0) {
      toast.error('Please select at least one battery');
      return;
    }

    createMutation.mutate({
      name,
      scope,
      batteries: scope === 'battery_specific' ? batteries : undefined,
    });
  };

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center gap-4">
          <Link to="/dashboard/sessions">
            <Button variant="ghost" size="icon">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Create Session</h1>
            <p className="text-muted-foreground">Create a new attendance session</p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Session Details</CardTitle>
            <CardDescription>Fill in the session information</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid gap-2">
                <Label htmlFor="name">Session Name *</Label>
                <Input
                  id="name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g., Monday Morning Formation"
                  required
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="scope">Scope *</Label>
                <Select
                  value={scope}
                  onValueChange={(value) => {
                    setScope(value);
                    if (value === 'unit_wide') {
                      setBatteries([]);
                    }
                  }}
                  required
                >
                  <SelectTrigger id="scope">
                    <SelectValue placeholder="Select scope" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="unit_wide">Unit Wide</SelectItem>
                    <SelectItem value="battery_specific">Battery Specific</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {scope === 'battery_specific' && (
                <div className="grid gap-2">
                  <Label>Batteries *</Label>
                  <div className="space-y-2">
                    {['HQ', 'Alpha', 'Bravo'].map((battery) => (
                      <div key={battery} className="flex items-center gap-2">
                        <Switch
                          id={`battery-${battery}`}
                          checked={batteries.includes(battery)}
                          onCheckedChange={(checked) => handleBatteryChange(battery, checked)}
                        />
                        <Label htmlFor={`battery-${battery}`} className="cursor-pointer">
                          {battery}
                        </Label>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <div className="flex justify-end gap-2 pt-4">
                <Button type="submit" disabled={createMutation.isPending}>
                  <Save className="mr-2 h-4 w-4" />
                  {createMutation.isPending ? 'Creating...' : 'Create Session'}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
}

