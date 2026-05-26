import { useMemo } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../ui/table';
import { Button } from '../ui/button';
import { Checkbox } from '../ui/checkbox';
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
  // Selection support (feature 004). When `selectable` is true, a leading
  // checkbox column is rendered with row + header semantics. `selectedIds`
  // is authoritative; `onToggleRow` toggles a single id; `onTogglePage`
  // takes the visible ids on this page (the header click target).
  // `disabledIds` are rows whose checkbox is rendered disabled (self /
  // system admin protection) — they are NEVER included by the header
  // checkbox.
  selectable?: boolean;
  selectedIds?: Set<string>;
  onToggleRow?: (userId: string) => void;
  onTogglePage?: (visibleIds: string[]) => void;
  disabledIds?: Set<string>;
}

export function UserTable({
  users,
  showActions = true,
  emptyMessage = 'No users found',
  onDelete,
  onMark,
  markingUserId,
  selectable = false,
  selectedIds,
  onToggleRow,
  onTogglePage,
  disabledIds,
}: UserTableProps) {
  const showMarkButton = !!onMark;
  const safeSelected = useMemo(
    () => selectedIds ?? new Set<string>(),
    [selectedIds],
  );
  const safeDisabled = useMemo(
    () => disabledIds ?? new Set<string>(),
    [disabledIds],
  );

  const extraKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const user of users) {
      const extras = 'extras' in user ? (user as UserProfile).extras : null;
      if (extras) {
        for (const key of Object.keys(extras)) {
          keys.add(key);
        }
      }
    }
    return Array.from(keys).sort();
  }, [users]);

  // The header checkbox toggles only the rows visible on this page that are
  // NOT in `disabledIds`. Indeterminate state when some-but-not-all visible
  // (eligible) rows are selected.
  const eligibleVisibleIds = useMemo(
    () => users.map((u) => u.id).filter((id) => !safeDisabled.has(id)),
    [users, safeDisabled],
  );
  const selectedVisibleCount = useMemo(
    () => eligibleVisibleIds.filter((id) => safeSelected.has(id)).length,
    [eligibleVisibleIds, safeSelected],
  );
  const headerState: boolean | 'indeterminate' =
    eligibleVisibleIds.length === 0
      ? false
      : selectedVisibleCount === 0
        ? false
        : selectedVisibleCount === eligibleVisibleIds.length
          ? true
          : 'indeterminate';

  if (!users || users.length === 0) {
    return (
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              {selectable && <TableHead className="w-10"></TableHead>}
              <TableHead>Name</TableHead>
              <TableHead className="hidden sm:table-cell">Rank</TableHead>
              <TableHead className="hidden sm:table-cell">Battery</TableHead>
              {showActions && <TableHead>Actions</TableHead>}
              {showMarkButton && <TableHead className="w-[70px]"></TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell
                colSpan={
                  3 +
                  extraKeys.length +
                  (selectable ? 1 : 0) +
                  (showActions ? 1 : 0) +
                  (showMarkButton ? 1 : 0)
                }
                className="text-center text-muted-foreground"
              >
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
            {selectable && (
              <TableHead className="w-10">
                <Checkbox
                  checked={headerState}
                  disabled={eligibleVisibleIds.length === 0}
                  onCheckedChange={() => onTogglePage?.(eligibleVisibleIds)}
                  aria-label="Select all on this page"
                  title="Select all on this page"
                />
              </TableHead>
            )}
            <TableHead>Name</TableHead>
            <TableHead className="hidden sm:table-cell">Rank</TableHead>
            <TableHead className="hidden sm:table-cell">Battery</TableHead>
            {extraKeys.map((key) => (
              <TableHead key={key} className="hidden sm:table-cell">{key}</TableHead>
            ))}
            {showActions && <TableHead>Actions</TableHead>}
            {showMarkButton && <TableHead className="w-[70px]"></TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((user) => {
            const extras = 'extras' in user ? (user as UserProfile).extras : null;
            const isDisabled = safeDisabled.has(user.id);
            const isChecked = safeSelected.has(user.id);
            return (
              <TableRow key={user.id} className="h-12">
                {selectable && (
                  <TableCell className="py-2">
                    <Checkbox
                      checked={isChecked}
                      disabled={isDisabled}
                      onCheckedChange={() => onToggleRow?.(user.id)}
                      aria-label={`Select ${user.fullName ?? user.id}`}
                      title={
                        isDisabled
                          ? 'You cannot delete your own account from here'
                          : undefined
                      }
                    />
                  </TableCell>
                )}
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
                {extraKeys.map((key) => (
                  <TableCell key={key} className="hidden sm:table-cell">
                    {extras?.[key] || ''}
                  </TableCell>
                ))}
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
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
