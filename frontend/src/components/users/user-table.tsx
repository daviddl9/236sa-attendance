import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../ui/table';
import { Button } from '../ui/button';
import { Link } from '@tanstack/react-router';
import { Pencil, Trash2, UserCheck } from 'lucide-react';
import type { UserInfo, UserProfile } from '../../lib/api-client';
import { StatusBadge } from '../status-badge';

interface UserTableProps {
  users: (UserInfo | UserProfile)[];
  showActions?: boolean;
  emptyMessage?: string;
  onDelete?: (userId: string) => void;
  onMark?: (userId: string) => void;
  markingUserId?: string;
}

export function UserTable({
  users,
  showActions = true,
  emptyMessage = 'No users found',
  onDelete,
  onMark,
  markingUserId,
}: UserTableProps) {
  const showMarkButton = !!onMark;

  if (!users || users.length === 0) {
    return (
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="hidden sm:table-cell">Rank</TableHead>
              <TableHead className="hidden sm:table-cell">Battery</TableHead>
              {showActions && <TableHead>Actions</TableHead>}
              {showMarkButton && <TableHead className="w-[70px]"></TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell colSpan={5} className="text-center text-muted-foreground">
                {emptyMessage}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    );
  }

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead className="hidden sm:table-cell">Rank</TableHead>
            <TableHead className="hidden sm:table-cell">Battery</TableHead>
            {showActions && <TableHead>Actions</TableHead>}
            {showMarkButton && <TableHead className="w-[70px]"></TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((user) => (
            <TableRow key={user.id} className="h-12">
              <TableCell className="py-2">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm">{user.fullName || 'N/A'}</span>
                  {'activeStatus' in user && user.activeStatus && (
                    <StatusBadge statusType={user.activeStatus.statusType} className="text-xs" />
                  )}
                </div>
                <div className="text-xs text-muted-foreground sm:hidden">
                  {user.rank || ''} · {user.battery || ''}
                </div>
              </TableCell>
              <TableCell className="hidden sm:table-cell">{user.rank || 'N/A'}</TableCell>
              <TableCell className="hidden sm:table-cell">{user.battery || 'N/A'}</TableCell>
              {showActions && (
                <TableCell className="py-2">
                  <div className="flex items-center gap-1">
                    <Link to="/dashboard/users/$userId" params={{ userId: user.id }}>
                      <Button variant="ghost" size="icon" className="h-8 w-8">
                        <Pencil className="h-4 w-4" />
                        <span className="sr-only">Edit</span>
                      </Button>
                    </Link>
                    {onDelete && (
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                        onClick={() => onDelete(user.id)}
                      >
                        <Trash2 className="h-4 w-4" />
                        <span className="sr-only">Delete</span>
                      </Button>
                    )}
                  </div>
                </TableCell>
              )}
              {showMarkButton && (
                <TableCell className="py-2">
                  <Button
                    size="sm"
                    className="h-8 px-2"
                    onClick={() => onMark(user.id)}
                    disabled={markingUserId === user.id}
                  >
                    <UserCheck className="h-4 w-4 sm:mr-1" />
                    <span className="hidden sm:inline">
                      {markingUserId === user.id ? '...' : 'Mark'}
                    </span>
                  </Button>
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

