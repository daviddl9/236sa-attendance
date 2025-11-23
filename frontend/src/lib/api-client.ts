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

export interface ChatMessage {
  role: string;
  content: string;
}

export interface ChatRequest {
  messages: ChatMessage[];
  model?: string;
}

export interface Subscription {
  id: string;
  createdAt: string;
  modifiedAt?: string | null;
  amount: number;
  currency: string;
  recurringInterval: string;
  status: string;
  currentPeriodStart: string;
  currentPeriodEnd: string;
  cancelAtPeriodEnd: boolean;
  canceledAt?: string | null;
  startedAt: string;
  endsAt?: string | null;
  endedAt?: string | null;
  customerId: string;
  productId: string;
  discountId?: string | null;
  checkoutId: string;
  customerCancellationReason?: string | null;
  customerCancellationComment?: string | null;
  metadata?: string | null;
  customFieldData?: string | null;
  userId?: string | null;
}

export interface SubscriptionResponse {
  subscription?: Subscription | null;
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

  // Chat endpoint
  async streamChat(messages: ChatMessage[]): Promise<Response> {
    const url = `${this.baseURL}/api/chat`;

    return fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'include',
      body: JSON.stringify({ messages }),
    });
  }

  // Subscription endpoint
  async getUserSubscription(): Promise<SubscriptionResponse> {
    return this.request<SubscriptionResponse>('/api/user/subscription');
  }
}

export const apiClient = new APIClient();
