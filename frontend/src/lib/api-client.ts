// API client for Go backend
const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

// Type definitions based on backend models
export interface User {
  id: string;
  name: string;
  email: string;
  emailVerified: boolean;
  image?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface AuthResponse {
  user: User;
  session: string;
}

export interface SignUpRequest {
  name: string;
  email: string;
  password: string;
}

export interface SignInRequest {
  email: string;
  password: string;
}

export interface SignOutResponse {
  success: boolean;
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
  async signUp(data: SignUpRequest): Promise<AuthResponse> {
    return this.request<AuthResponse>('/api/auth/sign-up', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

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
  async updateProfile(data: { name: string }): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/user/profile', {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }
}

export const apiClient = new APIClient();
