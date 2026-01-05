// API client for Go backend
const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

// Type definitions based on backend models
export interface User {
  id: string;
  fullName?: string | null;
  rank?: string | null;
  battery?: string | null;
  nricLast4?: string | null;
  dob?: string | null;
  isSuperadmin: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface AuthResponse {
  user: User;
  session: string;
}

export interface SignInRequest {
  identifier: string;
  password: string;
}

export interface RegisterUserRequest {
  fullName: string;
  rank: string;
  battery: string;
  nricLast4: string;
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
  nricLast4?: string;
  dob?: string;
}

export interface AttendanceSession {
  id: string;
  name: string;
  qrCode: string;
  scope: string;
  batteries: string[];
  status: string;
  createdBy: string;
  startTime: string;
  endTime?: string | null;
  closedAt?: string | null;
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

export interface BulkUploadResponse {
  success: number;
  failed: number;
  errors?: string[];
  users?: User[];
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
  async signIn(data: SignInRequest): Promise<AuthResponse> {
    return this.request<AuthResponse>('/api/auth/sign-in', {
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

  async getBulkDeleteCount(params: {
    search?: string;
    battery?: string;
    rank?: string;
  }): Promise<{ count: number }> {
    const queryParams = new URLSearchParams();
    if (params.search) queryParams.set('search', params.search);
    if (params.battery) queryParams.set('battery', params.battery);
    if (params.rank) queryParams.set('rank', params.rank);
    const query = queryParams.toString();
    return this.request<{ count: number }>(
      `/api/users/bulk/count${query ? `?${query}` : ''}`
    );
  }

  async bulkDeleteUsers(params: {
    search?: string;
    battery?: string;
    rank?: string;
  }): Promise<{ message: string; deletedCount: number }> {
    return this.request<{ message: string; deletedCount: number }>(
      '/api/users/bulk',
      {
        method: 'DELETE',
        body: JSON.stringify(params),
      }
    );
  }

  // Admin endpoints (superadmin only)
  async bulkUploadUsers(file: File): Promise<BulkUploadResponse> {
    const formData = new FormData();
    formData.append('file', file);

    const url = `${this.baseURL}/api/admin/users/bulk-upload`;
    const response = await fetch(url, {
      method: 'POST',
      credentials: 'include',
      body: formData,
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || response.statusText);
    }

    return response.json();
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
}

export const apiClient = new APIClient();
