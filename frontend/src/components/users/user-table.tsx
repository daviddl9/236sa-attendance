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
  const colCount = 3 + (showActions ? 1 : 0) + (showMarkButton ? 1 : 0);

  if (!users || users.length === 0) {
    return (
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Full Name</TableHead>
              <TableHead>Rank</TableHead>
              <TableHead>Battery</TableHead>
              {showActions && <TableHead>Actions</TableHead>}
              {showMarkButton && <TableHead>Actions</TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell colSpan={colCount} className="text-center text-muted-foreground">
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
            <TableHead>Full Name</TableHead>
            <TableHead>Rank</TableHead>
            <TableHead>Battery</TableHead>
            {showActions && <TableHead>Actions</TableHead>}
            {showMarkButton && <TableHead>Actions</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((user) => (
            <TableRow key={user.id}>
              <TableCell className="font-medium">
                {user.fullName || 'N/A'}
              </TableCell>
              <TableCell>{user.rank || 'N/A'}</TableCell>
              <TableCell>{user.battery || 'N/A'}</TableCell>
              {showActions && (
                <TableCell>
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
                <TableCell>
                  <Button
                    size="sm"
                    onClick={() => onMark(user.id)}
                    disabled={markingUserId === user.id}
                  >
                    <UserCheck className="mr-1 h-3 w-3" />
                    {markingUserId === user.id ? 'Marking...' : 'Mark'}
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

