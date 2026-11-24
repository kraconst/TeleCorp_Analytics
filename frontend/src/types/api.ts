export type UserRole = 'owner' | 'manager' | 'analyst';
export type ActiveUserRole = Exclude<UserRole, 'analyst'>;
export type AccountStatus = 'connecting' | 'active' | 'disconnected' | 'banned';
export type ChatStatus =
  | 'new_lead'
  | 'negotiation'
  | 'waiting_reply'
  | 'interested'
  | 'unhappy'
  | 'lost'
  | 'action_required';
export type StatusSource = 'auto' | 'manual';
export type ChatPriority = 'low' | 'normal' | 'high';
export type RiskLevel = 'low' | 'medium' | 'high';
export type AlertType = 'no_reply' | 'complaint' | 'refusal' | 'negative' | 'urgent';
export type MessageDirection = 'in' | 'out';
export type MediaType = 'photo' | 'document' | 'voice';

export interface Account {
  id: number;
  company_id: number;
  employee_id: number;
  phone: string;
  username: string;
  employee_name: string;
  telegram_user_id: number;
  status: AccountStatus;
  last_activity_at: string | null;
  created_at: string;
}

export interface Employee {
  id: number;
  company_id: number;
  name: string;
  position: string;
  is_active: boolean;
}

export interface Contact {
  id: number;
  company_id: number;
  telegram_user_id: number;
  name: string;
  username: string;
}

export interface Chat {
  id: number;
  account_id: number;
  contact_id: number;
  status: ChatStatus;
  status_source: StatusSource;
  priority: ChatPriority;
  last_message_at: string;
  awaiting_reply_since: string | null;
  unread_count: number;
}

export interface ChatSummary {
  chat_id: number;
  topic: string;
  agreements: string;
  next_step: string;
  open_questions: string;
  risk: RiskLevel;
  last_message_id: number;
  generated_at: string;
}

export interface ChatDetails extends Chat {
  contact?: Contact;
  summary?: ChatSummary;
}

export interface Message {
  id: number;
  chat_id: number;
  telegram_message_id: number;
  direction: MessageDirection;
  text: string;
  media_type: MediaType | null;
  sent_at: string;
}

export interface ChatDailyActivity {
  day: string;
  messages_in: number;
  messages_out: number;
}

export interface ChatAnalytics {
  chat_id: number;
  total_messages: number;
  messages_in: number;
  messages_out: number;
  media_messages: number;
  response_count: number;
  avg_response_seconds: number | null;
  max_response_seconds: number | null;
  unanswered_messages: number;
  awaiting_reply_since: string | null;
  first_message_at: string | null;
  last_message_at: string | null;
  daily_activity: ChatDailyActivity[];
}

export interface DashboardStatsRaw {
  messages_in: number;
  messages_out: number;
  active_chats: number;
  new_contacts: number;
  avg_response_time: number;
  unanswered_chats: number;
}

export interface DashboardStats extends Omit<DashboardStatsRaw, 'avg_response_time'> {
  avgResponseSeconds: number;
}

export interface AccountRatingRaw {
  account_id: number;
  username: string;
  employee_name: string;
  messages_in: number;
  messages_out: number;
  total_messages: number;
  active_chats: number;
  new_contacts: number;
  avg_response_time: number;
}

export interface AccountRating extends Omit<AccountRatingRaw, 'avg_response_time'> {
  avgResponseSeconds: number;
}

export interface Alert {
  id: number;
  company_id: number;
  chat_id: number;
  type: AlertType;
  created_at: string;
  resolved_at: string | null;
}

export interface CompanyRaw {
  id: number;
  name: string;
  timezone: string;
  no_reply_threshold: number;
  created_at: string;
}

export interface Company extends Omit<CompanyRaw, 'no_reply_threshold'> {
  noReplyThresholdMinutes: number;
}

export interface ChatListItem extends Chat {
  contact: Contact | null;
  account: Account | null;
  employee: Employee | null;
}

export interface ChatFilters {
  status?: ChatStatus;
  employeeId?: number;
  accountId?: number;
  query?: string;
  unanswered?: boolean;
  noReplyThresholdMinutes?: number;
}

export interface DateRange {
  from: string;
  to: string;
}

export interface ApiRequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
  timeoutMs?: number;
}
