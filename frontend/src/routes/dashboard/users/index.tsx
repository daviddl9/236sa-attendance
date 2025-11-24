import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../../../lib/api-client';
import DashboardLayout from '../../../components/dashboard/layout';
import { Button } from '../../../components/ui/button';
import { Input } from '../../../components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../../components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../../components/ui/table';
import { useState } from 'react';
import { Plus, Search, Upload } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '../../../lib/auth-context';
import { Link } from '@tanstack/react-router';

export const Route = createFileRoute('/dashboard/users/')({
  component: UsersPage,
});

function UsersPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [batteryFilter, setBatteryFilter] = useState('');
  const [rankFilter, setRankFilter] = useState('');

  const isSuperadmin = user?.isSuperadmin || false;

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['users', page, search, batteryFilter, rankFilter],
    queryFn: () =>
      apiClient.listUsers({
        page,
        limit: 20,
        search: search || undefined,
        battery: batteryFilter || undefined,
        rank: rankFilter || undefined,
      }),
  });

  const handleBulkUpload = () => {
    navigate({ to: '/dashboard/users/bulk-upload' });
  };

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-4 p-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Users</h1>
            <p className="text-muted-foreground">
              Manage users and their information
            </p>
          </div>
          {isSuperadmin && (
            <div className="flex gap-2">
              <Button onClick={handleBulkUpload} variant="outline">
                <Upload className="mr-2 h-4 w-4" />
                Bulk Upload
              </Button>
            </div>
          )}
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <div className="relative">
              <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by name..."
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setPage(1);
                }}
                className="pl-8"
              />
            </div>
          </div>
          <Select
            value={batteryFilter}
            onValueChange={(value) => {
              setBatteryFilter(value);
              setPage(1);
            }}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="All Batteries" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">All Batteries</SelectItem>
              <SelectItem value="HQ">HQ</SelectItem>
              <SelectItem value="Alpha">Alpha</SelectItem>
              <SelectItem value="Bravo">Bravo</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={rankFilter}
            onValueChange={(value) => {
              setRankFilter(value);
              setPage(1);
            }}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="All Ranks" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">All Ranks</SelectItem>
              <SelectItem value="REC">REC</SelectItem>
              <SelectItem value="PTE">PTE</SelectItem>
              <SelectItem value="LCP">LCP</SelectItem>
              <SelectItem value="CPL">CPL</SelectItem>
              <SelectItem value="CFC">CFC</SelectItem>
              <SelectItem value="3SG">3SG</SelectItem>
              <SelectItem value="2SG">2SG</SelectItem>
              <SelectItem value="1SG">1SG</SelectItem>
              <SelectItem value="SSG">SSG</SelectItem>
              <SelectItem value="MSG">MSG</SelectItem>
              <SelectItem value="3WO">3WO</SelectItem>
              <SelectItem value="2WO">2WO</SelectItem>
              <SelectItem value="1WO">1WO</SelectItem>
              <SelectItem value="MWO">MWO</SelectItem>
              <SelectItem value="SWO">SWO</SelectItem>
              <SelectItem value="2LT">2LT</SelectItem>
              <SelectItem value="LTA">LTA</SelectItem>
              <SelectItem value="CPT">CPT</SelectItem>
              <SelectItem value="MAJ">MAJ</SelectItem>
              <SelectItem value="LTC">LTC</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <div className="text-muted-foreground">Loading...</div>
          </div>
        ) : (
          <>
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Full Name</TableHead>
                    <TableHead>Rank</TableHead>
                    <TableHead>Battery</TableHead>
                    <TableHead>Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data?.users.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={4} className="text-center text-muted-foreground">
                        No users found
                      </TableCell>
                    </TableRow>
                  ) : (
                    data?.users.map((user) => (
                      <TableRow key={user.id}>
                        <TableCell className="font-medium">
                          {user.fullName || 'N/A'}
                        </TableCell>
                        <TableCell>{user.rank || 'N/A'}</TableCell>
                        <TableCell>{user.battery || 'N/A'}</TableCell>
                        <TableCell>
                          <Link to="/dashboard/users/$userId" params={{ userId: user.id }}>
                            <Button variant="ghost" size="sm">
                              View
                            </Button>
                          </Link>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>

            {data && data.total > 20 && (
              <div className="flex items-center justify-between">
                <div className="text-sm text-muted-foreground">
                  Showing {(page - 1) * 20 + 1} to {Math.min(page * 20, data.total)} of{' '}
                  {data.total} users
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page === 1}
                  >
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => setPage((p) => p + 1)}
                    disabled={page * 20 >= data.total}
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </DashboardLayout>
  );
}

