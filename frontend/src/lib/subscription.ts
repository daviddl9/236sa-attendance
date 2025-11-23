import { apiClient } from "./api-client";

export type SubscriptionDetails = {
  id: string;
  productId: string;
  status: string;
  amount: number;
  currency: string;
  recurringInterval: string;
  currentPeriodStart: Date;
  currentPeriodEnd: Date;
  cancelAtPeriodEnd: boolean;
  canceledAt: Date | null;
  organizationId: string | null;
};

export type SubscriptionDetailsResult = {
  hasSubscription: boolean;
  subscription?: SubscriptionDetails;
  error?: string;
  errorType?: "CANCELED" | "EXPIRED" | "GENERAL";
};

export async function getSubscriptionDetails(): Promise<SubscriptionDetailsResult> {
  try {
    const user = await apiClient.getSession();

    if (!user?.id) {
      return { hasSubscription: false };
    }

    // Get subscription from API instead of direct DB access
    const subscriptionData = await apiClient.getUserSubscription();
    
    if (!subscriptionData || !subscriptionData.subscription) {
      return { hasSubscription: false };
    }

    const sub = subscriptionData.subscription;
    
    if (!sub) {
      return { hasSubscription: false };
    }

    const now = new Date();
    const isExpired = new Date(sub.currentPeriodEnd) < now;
    const isCanceled = sub.status === "canceled";
    const isActive = sub.status === "active" && !isExpired;

    if (!isActive) {
      return {
        hasSubscription: true,
        subscription: {
          id: sub.id,
          productId: sub.productId,
          status: sub.status,
          amount: sub.amount,
          currency: sub.currency,
          recurringInterval: sub.recurringInterval,
          currentPeriodStart: new Date(sub.currentPeriodStart),
          currentPeriodEnd: new Date(sub.currentPeriodEnd),
          cancelAtPeriodEnd: sub.cancelAtPeriodEnd,
          canceledAt: sub.canceledAt ? new Date(sub.canceledAt) : null,
          organizationId: null,
        },
        error: isCanceled ? "Subscription has been canceled" : isExpired ? "Subscription has expired" : "Subscription is not active",
        errorType: isCanceled ? "CANCELED" : isExpired ? "EXPIRED" : "GENERAL",
      };
    }

    return {
      hasSubscription: true,
      subscription: {
        id: sub.id,
        productId: sub.productId,
        status: sub.status,
        amount: sub.amount,
        currency: sub.currency,
        recurringInterval: sub.recurringInterval,
        currentPeriodStart: new Date(sub.currentPeriodStart),
        currentPeriodEnd: new Date(sub.currentPeriodEnd),
        cancelAtPeriodEnd: sub.cancelAtPeriodEnd,
        canceledAt: sub.canceledAt ? new Date(sub.canceledAt) : null,
        organizationId: null,
      },
    };
  } catch (error) {
    console.error("Error fetching subscription details:", error);
    return {
      hasSubscription: false,
      error: "Failed to load subscription details",
      errorType: "GENERAL",
    };
  }
}

// Simple helper to check if user has an active subscription
export async function isUserSubscribed(): Promise<boolean> {
  const result = await getSubscriptionDetails();
  return result.hasSubscription && result.subscription?.status === "active";
}

// Helper to check if user has access to a specific product/tier
export async function hasAccessToProduct(productId: string): Promise<boolean> {
  const result = await getSubscriptionDetails();
  return (
    result.hasSubscription &&
    result.subscription?.status === "active" &&
    result.subscription?.productId === productId
  );
}

// Helper to get user's current subscription status
export async function getUserSubscriptionStatus(): Promise<"active" | "canceled" | "expired" | "none"> {
  const result = await getSubscriptionDetails();
  
  if (!result.hasSubscription) {
    return "none";
  }
  
  if (result.subscription?.status === "active") {
    return "active";
  }
  
  if (result.errorType === "CANCELED") {
    return "canceled";
  }
  
  if (result.errorType === "EXPIRED") {
    return "expired";
  }
  
  return "none";
}