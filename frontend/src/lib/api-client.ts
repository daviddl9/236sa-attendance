// API client for Go backend. Empty string means same-origin (the Vite dev
// server proxies /api, and production serves the SPA and API from one origin).
export const API_URL = import.meta.env.VITE_API_URL || '';

// Access tiers (must match backend models.AccessTier)
export type AccessTier = 1 | 2 | 3 | 4;

// Type definitions based on backend models
export interface User {
  id: string;
  fullName?: string | null;
  rank?: string | null;
  battery?: string | null;
  nricLast5?: string | null;
  dob?: string | null;
  extras?: Record<string, string> | null;
  tierOverride?: 2 | 3 | null;
  verified: boolean;
  isSuperadmin: boolean;
  tier: AccessTier; // computed by server
  createdAt: string;
  updatedAt: string;
}

export type SignInOutcome = 'authenticated' | 'pending_approval';

export interface SignInAuthenticatedResponse {
  outcome: 'authenticated';
  user: User;
  session: string;
}

export interface SignInPendingApprovalResponse {
  outcome: 'pending_approval';
}

export type SignInResponse =
  | SignInAuthenticatedResponse
  | SignInPendingApprovalResponse;

// Kept for callers that destructure { user, session } from sign-in directly.
export type AuthResponse = SignInAuthenticatedResponse;

export interface SignInRequest {
  identifier: string;
  password?: string;
  dob?: string;
}

export interface SignUpRequest {
  username: string;
  password: string;
  confirmPassword: string;
  fullName: string;
  rank: string;
  battery: string;
}

export interface RegisterUserRequest {
  fullName: string;
  rank: string;
  battery: string;
  nricLast5: string;
  dob: string;
}

export interface RegisterUserResponse {
  user: User;
  session: string;
}

export interface SignOutResponse {
  success: boolean;
}

export interface UserProfile extends User {
  username?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface UserListResponse {
  users: UserProfile[];
  total: number;
  page: number;
  limit: number;
}

export interface UpdateUserRequest {
  fullName?: string;
  rank?: string;
  battery?: string;
  nricLast5?: string;
  dob?: string;
  tierOverride?: 2 | 3 | null;
}

// Single-user create (feature 003) — superadmin only.
export interface CreateUserRequest {
  fullName: string;
  rank: string;
  battery: string;
  nricLast5: string;
  dob?: string;
  extras?: Record<string, string>;
}

// 409 response shape from POST /api/users when a user already exists.
export interface CreateUserConflictDetails {
  error: 'user_exists';
  existingUserId: string;
  verified: boolean;
  fullName: string;
}

export class CreateUserConflictError extends Error {
  details: CreateUserConflictDetails;
  constructor(details: CreateUserConflictDetails) {
    super(`User already exists: ${details.fullName}`);
    this.name = 'CreateUserConflictError';
    this.details = details;
  }
}

// Bulk-delete (feature 004) — selection-based, superadmin only.
export interface BulkDeleteRequest {
  userIds: string[];
}

export type BulkDeleteSkipReason = 'self' | 'system_admin' | 'not_found';

export interface BulkDeleteSkip {
  id: string;
  reason: BulkDeleteSkipReason;
}

export interface BulkDeleteFailure {
  id: string;
  code: string;
}

export interface BulkDeleteSummary {
  requested: number;
  deleted: number;
  skipped: number;
  failed: number;
}

export interface BulkDeleteResponse {
  deleted: string[];
  skipped: BulkDeleteSkip[];
  failed: BulkDeleteFailure[];
  summary: BulkDeleteSummary;
}

// Pending registration (admin approval)
export interface PendingRegistration {
  id: string;
  username: string;
  fullName: string;
  rank: string;
  battery: string;
  createdAt: string;
}

export interface PendingRegistrationsResponse {
  registrations: PendingRegistration[];
  total: number;
}

export interface BulkApprovalResult {
  id: string;
  success: boolean;
  error?: string;
}

export interface BulkApprovalResponse {
  results: BulkApprovalResult[];
  approved: number;
  failed: number;
}

export interface ProvisionCredentialsResponse {
  username: string;
  temporaryPassword: string;
}

export interface RegistrationCandidate {
  id: string;
  fullName: string;
  rank: string;
  battery: string;
  score: number;
  strong: boolean;
  mismatchReasons: string[];
  alreadyClaimed: boolean;
  claimedBy?: string;
  selectable: boolean;
  preselected: boolean;
}

export interface RegistrationCandidatesResponse {
  candidates: RegistrationCandidate[];
}

export interface TelegramPairingRecord {
  telegramId: number;
  userId: string;
  fullName: string;
  selfConfirmed: boolean;
  confirmedBy?: string;
  createdAt: string;
}

export interface TelegramPairingAttemptRecord {
  telegramId: number;
  outcome: 'proposed' | 'confirmed' | 'refused_conflict' | 'no_match';
  userId?: string;
  fullName?: string;
  createdAt: string;
}

export interface TelegramPairingRequestRecord {
  telegramId: number;
  displayName: string;
  attemptId: string;
  createdAt: string;
  candidates: RegistrationCandidate[];
}

export interface TelegramPairingReviewResponse {
  pairings: TelegramPairingRecord[];
  attempts: TelegramPairingAttemptRecord[];
  requests: TelegramPairingRequestRecord[];
}

// Custom participant session (Feature 004)
export interface ParticipantMatch {
  userId: string;
  fullName: string;
  rank?: string | null;
  battery?: string | null;
}

export interface UnmatchedRow {
  rowNum: number;
  fullName: string;
  nricLast5?: string;
  reason: string;
}

export interface CustomSessionPreviewResponse {
  matched: ParticipantMatch[];
  unmatched: UnmatchedRow[];
}

export interface CreateCustomSessionRequest {
  name: string;
  participantIds: string[];
  endTime?: string | null;
}

// Reusable participant groups
export interface ParticipantGroup {
  id: string;
  name: string;
  createdBy: string;
  memberCount?: number;
  createdAt: string;
  updatedAt: string;
}

export interface GroupResponse {
  id: string;
  name: string;
  createdBy: string;
  memberCount?: number;
  createdAt: string;
  updatedAt: string;
  memberIds?: string[];
}

export interface CreateGroupRequest {
  name: string;
  participantIds: string[];
}

export interface GroupListResponse {
  groups: ParticipantGroup[];
}

export interface AttendanceSession {
  id: string;
  name: string;
  qrCode?: string;
  scope: string;
  batteries: string[];
  status: string;
  createdBy: string;
  startTime: string;
  endTime?: string | null;
  closedAt?: string | null;
  telegramLink?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateSessionRequest {
  name: string;
  scope: string;
  batteries?: string[];
}

export type SessionResponse = AttendanceSession;

export interface AttendanceRecord {
  id: string;
  sessionId: string;
  userId: string;
  markedAt: string;
  markingMethod: string;
  markedBy?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface MarkAttendanceRequest {
  qrData: string;
}

export interface ManualMarkRequest {
  userIds: string[];
}

export interface SessionAnalytics {
  totalUsers: number;
  presentCount: number;
  attendancePercentage: number;
  missingUsers: UserInfo[];
  presentUsers: UserInfo[];
  byBattery: Record<string, BatteryStats>;
  byRank: Record<string, RankStats>;
}

export interface StatusInfo {
  statusType: StatusType;
  displayName: string;
}

export interface UserInfo {
  id: string;
  fullName?: string | null;
  rank?: string | null;
  battery?: string | null;
  activeStatus?: StatusInfo | null;
}

export interface BatteryStats {
  total: number;
  present: number;
}

export interface RankStats {
  total: number;
  present: number;
}

export interface UserReport {
  user: UserInfo;
  totalSessions: number;
  attended: number;
  attendanceRate: number;
  recentSessions: SessionRecord[];
}

export interface SessionRecord {
  sessionId: string;
  sessionName: string;
  markedAt: string;
  markingMethod: string;
}

export interface BatteryReportResponse {
  battery: string;
  sessions: BatterySessionStats[];
}

export interface BatterySessionStats {
  sessionId: string;
  sessionName: string;
  startTime: string;
  totalUsers: number;
  presentCount: number;
  attendancePercentage: number;
}

export interface BulkUploadRow {
  fullName: string;
  rank: string;
  battery: string;
  nricLast5: string;
  extras?: Record<string, string>;
}

export interface BulkUploadResponse {
  success: number;
  failed: number;
  skipped?: number;
  errors?: string[];
  message?: string;
  users?: User[];
}

// Document-import (feature 002) types
export type ImportMatchStatus = 'new' | 'existing_match' | 'invalid';

export interface ImportRowCounts {
  created: number;
  updated: number;
  skipped: number;
  failed: number;
}

export interface ImportPreviewedRow {
  fullName: string;
  rank?: string;
  battery?: string;
  nricLast5: string;
  extras?: Record<string, string>;
  sourceSnippet?: string;
  confidence: number;
  matchStatus: ImportMatchStatus;
  existingId?: string;
  validationMsg?: string;
}

export interface ImportPreviewResponse {
  importId: string;
  filename: string;
  rowCounts: ImportRowCounts;
  rows: ImportPreviewedRow[];
}

export type ImportMergeMode = 'fill_gaps' | 'override';

export interface ImportCommitRow {
  fullName: string;
  rank?: string;
  battery?: string;
  nricLast5: string;
}

export interface ImportCommitRequest {
  importId: string;
  mergeMode: ImportMergeMode;
  rows: ImportCommitRow[];
}

export interface ImportCommitResponse {
  importId: string;
  rowCounts: ImportRowCounts;
}

// Status types
export type StatusType = 'stay_out' | 'off_pass' | 'mc' | 'course' | 'other';

export interface UserStatusDate {
  id?: string;
  statusId?: string;
  startDate: string;
  startTime: string;
  endDate: string;
  endTime: string;
}

export interface UserStatus {
  id: string;
  userId: string;
  statusType: StatusType;
  dailyAfterTime?: string | null;
  startDate?: string | null;
  endDate?: string | null;
  notes?: string | null;
  dates?: UserStatusDate[];
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface UserStatusWithUser extends UserStatus {
  userFullName?: string | null;
  userRank?: string | null;
  userBattery?: string | null;
}

export interface StatusListResponse {
  statuses: UserStatusWithUser[];
  total: number;
  page: number;
  limit: number;
}

export interface CreateStatusRequest {
  userId: string;
  statusType: StatusType;
  dailyAfterTime?: string;
  startDate?: string;
  endDate?: string;
  notes?: string;
  dates?: Omit<UserStatusDate, 'id' | 'statusId'>[];
}

export interface UpdateStatusRequest {
  statusType?: StatusType;
  dailyAfterTime?: string;
  startDate?: string;
  endDate?: string;
  notes?: string;
  dates?: Omit<UserStatusDate, 'id' | 'statusId'>[];
}

export const STATUS_DISPLAY_CONFIG: Record<
  StatusType,
  { label: string; color: string }
> = {
  stay_out: { label: 'Stay Out', color: 'bg-orange-500' },
  off_pass: { label: 'Off Pass', color: 'bg-blue-500' },
  mc: { label: 'MC', color: 'bg-red-500' },
  course: { label: 'Course', color: 'bg-purple-500' },
  other: { label: 'Other', color: 'bg-gray-500' },
};

// Error type with status code
export class APIError extends Error {
  status: number;
  statusText: string;

  constructor(message: string, status: number, statusText: string) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.statusText = statusText;
  }
}

export class APIClient {
  private baseURL: string;

  constructor(baseURL: string = API_URL) {
    this.baseURL = baseURL;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseURL}${endpoint}`;

    const config: RequestInit = {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      credentials: 'include', // Include cookies for session management
    };

    const response = await fetch(url, config);

    if (!response.ok) {
      const errorText = await response.text();
      throw new APIError(errorText || response.statusText, response.status, response.statusText);
    }

    return response.json();
  }

  // Auth endpoints
  async signIn(data: SignInRequest): Promise<SignInResponse> {
    return this.request<SignInResponse>('/api/auth/sign-in', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async signUp(data: SignUpRequest): Promise<SignInResponse> {
    return this.request<SignInResponse>('/api/auth/sign-up', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async signOut(): Promise<SignOutResponse> {
    return this.request<SignOutResponse>('/api/auth/sign-out', {
      method: 'POST',
    });
  }

  async registerUser(data: RegisterUserRequest): Promise<RegisterUserResponse> {
    return this.request<RegisterUserResponse>('/api/users/register', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async getSession(): Promise<User> {
    return this.request<User>('/api/auth/session');
  }

  // User profile endpoints
  async getProfile(): Promise<UserProfile> {
    return this.request<UserProfile>('/api/user/profile');
  }

  async updateProfile(data: UpdateUserRequest): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/user/profile', {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  // User management endpoints (commander+)
  async listUsers(params?: {
    page?: number;
    limit?: number;
    search?: string;
    battery?: string;
    rank?: string;
  }): Promise<UserListResponse> {
    const queryParams = new URLSearchParams();
    if (params?.page) queryParams.set('page', params.page.toString());
    if (params?.limit) queryParams.set('limit', params.limit.toString());
    if (params?.search) queryParams.set('search', params.search);
    if (params?.battery) queryParams.set('battery', params.battery);
    if (params?.rank) queryParams.set('rank', params.rank);

    const query = queryParams.toString();
    return this.request<UserListResponse>(`/api/users${query ? `?${query}` : ''}`);
  }

  async getUser(id: string): Promise<UserProfile> {
    return this.request<UserProfile>(`/api/users/${id}`);
  }

  async updateUser(id: string, data: UpdateUserRequest): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/users/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async deleteUser(id: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/users/${id}`, {
      method: 'DELETE',
    });
  }

  // Create a single user (superadmin only). On 409, the server returns a
  // structured conflict body; we surface it as CreateUserConflictError so
  // callers can deep-link to the existing record.
  async createUser(data: CreateUserRequest): Promise<UserProfile> {
    const url = `${this.baseURL}/api/users`;
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(data),
    });

    if (response.status === 409) {
      const body = (await response.json()) as CreateUserConflictDetails;
      throw new CreateUserConflictError(body);
    }
    if (!response.ok) {
      const errorText = await response.text();
      throw new APIError(errorText || response.statusText, response.status, response.statusText);
    }
    return response.json() as Promise<UserProfile>;
  }

  // Selection-based bulk delete (superadmin only). Returns a per-id
  // outcome with `deleted` / `skipped` / `failed` arrays.
  async bulkDeleteUsers(data: BulkDeleteRequest): Promise<BulkDeleteResponse> {
    return this.request<BulkDeleteResponse>('/api/users/bulk-delete', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  // Admin endpoints (superadmin only)
  async bulkCreateUsers(users: BulkUploadRow[]): Promise<BulkUploadResponse> {
    return this.request<BulkUploadResponse>('/api/admin/users/bulk-create', {
      method: 'POST',
      body: JSON.stringify({ users }),
    });
  }

  async previewImportDocument(file: File): Promise<ImportPreviewResponse> {
    const formData = new FormData();
    formData.append('file', file);

    const response = await fetch(
      `${this.baseURL}/api/admin/users/import-document/preview`,
      { method: 'POST', body: formData, credentials: 'include' },
    );

    if (!response.ok) {
      let message = response.statusText;
      try {
        const body = (await response.json()) as { error?: string };
        if (body.error) message = body.error;
      } catch {
        // Body wasn't JSON — fall back to statusText.
      }
      throw new APIError(message, response.status, response.statusText);
    }

    return response.json() as Promise<ImportPreviewResponse>;
  }

  async commitImport(req: ImportCommitRequest): Promise<ImportCommitResponse> {
    const response = await fetch(
      `${this.baseURL}/api/admin/users/import-document/commit`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
        credentials: 'include',
      },
    );

    if (!response.ok) {
      let message = response.statusText;
      try {
        const body = (await response.json()) as { error?: string };
        if (body.error) message = body.error;
      } catch {
        // fall back to statusText
      }
      throw new APIError(message, response.status, response.statusText);
    }

    return response.json() as Promise<ImportCommitResponse>;
  }

  // Session endpoints (commander+)
  async createSession(data: CreateSessionRequest): Promise<SessionResponse> {
    return this.request<SessionResponse>('/api/sessions', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async listSessions(params?: {
    status?: string;
    battery?: string;
    from?: string;
    to?: string;
  }): Promise<AttendanceSession[]> {
    const queryParams = new URLSearchParams();
    if (params?.status) queryParams.set('status', params.status);
    if (params?.battery) queryParams.set('battery', params.battery);
    if (params?.from) queryParams.set('from', params.from);
    if (params?.to) queryParams.set('to', params.to);

    const query = queryParams.toString();
    return this.request<AttendanceSession[]>(`/api/sessions${query ? `?${query}` : ''}`);
  }

  async getActiveSessions(): Promise<AttendanceSession[]> {
    return this.request<AttendanceSession[]>('/api/sessions/active');
  }

  async getSessionById(id: string): Promise<AttendanceSession> {
    return this.request<AttendanceSession>(`/api/sessions/${id}`);
  }


  async closeSession(id: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/sessions/${id}/close`, {
      method: 'PUT',
    });
  }

  async deleteSession(id: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/sessions/${id}`, {
      method: 'DELETE',
    });
  }

  // Attendance endpoints
  async markAttendance(data: MarkAttendanceRequest): Promise<{ message: string; recordId: string }> {
    return this.request<{ message: string; recordId: string }>('/api/attendance/mark', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async manualMarkAttendance(sessionId: string, data: ManualMarkRequest): Promise<{
    message: string;
    successCount: number;
    errors: string[];
  }> {
    return this.request(`/api/sessions/${sessionId}/attendance/manual`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async removeAttendance(sessionId: string, userId: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/sessions/${sessionId}/attendance/${userId}`, {
      method: 'DELETE',
    });
  }

  // Reports endpoints (commander+)
  async getSessionAnalytics(sessionId: string): Promise<SessionAnalytics> {
    return this.request<SessionAnalytics>(`/api/reports/sessions/${sessionId}/analytics`);
  }

  async getMissingUsers(sessionId: string): Promise<UserInfo[]> {
    return this.request<UserInfo[]>(`/api/reports/sessions/${sessionId}/missing`);
  }

  async getUserReport(userId: string): Promise<UserReport> {
    return this.request<UserReport>(`/api/reports/user/${userId}`);
  }

  async getBatteryReport(battery: string): Promise<BatteryReportResponse> {
    return this.request<BatteryReportResponse>(`/api/reports/battery/${battery}`);
  }

  // Export endpoints
  async exportSessionCSV(sessionId: string, battery?: string): Promise<Blob> {
    let url = `${this.baseURL}/api/sessions/${sessionId}/export/csv`;
    if (battery) {
      url += `?battery=${encodeURIComponent(battery)}`;
    }
    const response = await fetch(url, {
      credentials: 'include',
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || response.statusText);
    }

    return response.blob();
  }

  async exportSessionExcel(sessionId: string, battery?: string): Promise<Blob> {
    let url = `${this.baseURL}/api/sessions/${sessionId}/export/excel`;
    if (battery) {
      url += `?battery=${encodeURIComponent(battery)}`;
    }
    const response = await fetch(url, {
      credentials: 'include',
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || response.statusText);
    }

    return response.blob();
  }

  // Status endpoints (superadmin only)
  async listStatuses(params?: {
    page?: number;
    limit?: number;
    statusType?: StatusType;
    active?: boolean;
    userId?: string;
  }): Promise<StatusListResponse> {
    const queryParams = new URLSearchParams();
    if (params?.page) queryParams.set('page', params.page.toString());
    if (params?.limit) queryParams.set('limit', params.limit.toString());
    if (params?.statusType) queryParams.set('statusType', params.statusType);
    if (params?.active !== undefined)
      queryParams.set('active', params.active.toString());
    if (params?.userId) queryParams.set('userId', params.userId);

    const query = queryParams.toString();
    return this.request<StatusListResponse>(
      `/api/statuses${query ? `?${query}` : ''}`
    );
  }

  async createStatus(data: CreateStatusRequest): Promise<UserStatus> {
    return this.request<UserStatus>('/api/statuses', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async getStatus(id: string): Promise<UserStatusWithUser> {
    return this.request<UserStatusWithUser>(`/api/statuses/${id}`);
  }

  async updateStatus(
    id: string,
    data: UpdateStatusRequest
  ): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/statuses/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async deleteStatus(id: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/statuses/${id}`, {
      method: 'DELETE',
    });
  }

  // Registration approval (superadmin only)
  async listPendingRegistrations(battery?: string): Promise<PendingRegistrationsResponse> {
    const query = battery ? `?battery=${encodeURIComponent(battery)}` : '';
    return this.request<PendingRegistrationsResponse>(`/api/admin/registrations${query}`);
  }

  async bulkApproveRegistrations(registrationIds: string[]): Promise<BulkApprovalResponse> {
    return this.request<BulkApprovalResponse>('/api/admin/registrations/bulk-approve', {
      method: 'POST',
      body: JSON.stringify({ registrationIds }),
    });
  }

  async listRegistrationCandidates(id: string): Promise<RegistrationCandidatesResponse> {
    return this.request<RegistrationCandidatesResponse>(`/api/admin/registrations/${id}/candidates`);
  }

  async approveRegistration(
    id: string,
    data: { mode: 'link'; userId: string } | { mode: 'create'; acknowledgeStrongMatch?: boolean }
  ): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/admin/registrations/${id}/approve`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async rejectRegistration(id: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/admin/registrations/${id}/reject`, {
      method: 'POST',
    });
  }

  async listTelegramPairings(): Promise<TelegramPairingReviewResponse> {
    return this.request<TelegramPairingReviewResponse>('/api/admin/telegram/pairings');
  }

  async confirmTelegramPairing(telegramId: number, userId: string): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/admin/telegram/pairings/${telegramId}/confirm`, {
      method: 'POST',
      body: JSON.stringify({ userId }),
    });
  }

  async unpairTelegramAccount(telegramId: number): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/admin/telegram/pairings/${telegramId}`, {
      method: 'DELETE',
    });
  }

  async provisionUserCredentials(id: string, username: string): Promise<ProvisionCredentialsResponse> {
    return this.request<ProvisionCredentialsResponse>(`/api/admin/users/${id}/credentials`, {
      method: 'POST',
      body: JSON.stringify({ username }),
    });
  }

  // Custom participant sessions (Tier 3+)
  async previewCustomSession(file: File): Promise<CustomSessionPreviewResponse> {
    const formData = new FormData();
    formData.append('file', file);

    const response = await fetch(`${this.baseURL}/api/sessions/custom/preview`, {
      method: 'POST',
      body: formData,
      credentials: 'include',
    });

    if (!response.ok) {
      const text = await response.text();
      throw new APIError(text || response.statusText, response.status, response.statusText);
    }

    return response.json() as Promise<CustomSessionPreviewResponse>;
  }

  async createCustomSession(data: CreateCustomSessionRequest): Promise<SessionResponse> {
    return this.request<SessionResponse>('/api/sessions/custom/create', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  // Reusable participant groups
  async listGroups(): Promise<GroupListResponse> {
    return this.request<GroupListResponse>('/api/groups', { method: 'GET' });
  }

  async getGroup(id: string): Promise<GroupResponse> {
    return this.request<GroupResponse>(`/api/groups/${id}`, { method: 'GET' });
  }

  async createGroup(data: CreateGroupRequest): Promise<GroupResponse> {
    return this.request<GroupResponse>('/api/groups', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deleteGroup(id: string): Promise<void> {
    const response = await fetch(`${this.baseURL}/api/groups/${id}`, {
      method: 'DELETE',
      credentials: 'include',
    });
    if (!response.ok) {
      const text = await response.text();
      throw new APIError(text || response.statusText, response.status, response.statusText);
    }
  }

  async previewGroupFromExcel(file: File, password?: string): Promise<CustomSessionPreviewResponse> {
    const formData = new FormData();
    formData.append('file', file);
    if (password) formData.append('password', password);

    const response = await fetch(`${this.baseURL}/api/groups/preview`, {
      method: 'POST',
      body: formData,
      credentials: 'include',
    });

    if (!response.ok) {
      const text = await response.text();
      throw new APIError(text || response.statusText, response.status, response.statusText);
    }

    return response.json() as Promise<CustomSessionPreviewResponse>;
  }

  async createSessionFromGroup(groupId: string, data: CreateCustomSessionRequest): Promise<SessionResponse> {
    return this.request<SessionResponse>(`/api/groups/${groupId}/sessions`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async saveSessionAsGroup(sessionId: string, data: CreateGroupRequest): Promise<GroupResponse> {
    return this.request<GroupResponse>(`/api/sessions/${sessionId}/group`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async duplicateSession(sessionId: string, data: CreateCustomSessionRequest): Promise<SessionResponse> {
    return this.request<SessionResponse>(`/api/sessions/${sessionId}/duplicate`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }
}

export const apiClient = new APIClient();
