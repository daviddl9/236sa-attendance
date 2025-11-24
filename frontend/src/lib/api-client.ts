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
  sessionType: string;
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
  sessionType: string;
  scope: string;
  batteries?: string[];
  startTime: string;
  endTime?: string | null;
}

export interface SessionResponse extends AttendanceSession {
  qrCodeImage: string;
}

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
  byBattery: Record<string, BatteryStats>;
  byRank: Record<string, RankStats>;
}

export interface UserInfo {
  id: string;
  fullName?: string | null;
  rank?: string | null;
  battery?: string | null;
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
      const error = await response.text();
      throw new Error(error || response.statusText);
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

  async getSessionQR(id: string): Promise<Blob> {
    const url = `${this.baseURL}/api/sessions/${id}/qr`;
    const response = await fetch(url, {
      credentials: 'include',
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || response.statusText);
    }

    return response.blob();
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
  async exportSessionCSV(sessionId: string): Promise<Blob> {
    const url = `${this.baseURL}/api/sessions/${sessionId}/export/csv`;
    const response = await fetch(url, {
      credentials: 'include',
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || response.statusText);
    }

    return response.blob();
  }

  async exportSessionExcel(sessionId: string): Promise<Blob> {
    const url = `${this.baseURL}/api/sessions/${sessionId}/export/excel`;
    const response = await fetch(url, {
      credentials: 'include',
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || response.statusText);
    }

    return response.blob();
  }
}

export const apiClient = new APIClient();
