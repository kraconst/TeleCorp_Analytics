import type {
  AccountRating,
  AccountRatingRaw,
  Company,
  CompanyRaw,
  DashboardStats,
  DashboardStatsRaw,
} from '@/types/api';

const NANOSECONDS_PER_SECOND = 1_000_000_000;
const SECONDS_PER_MINUTE = 60;

export function durationToSeconds(value: number): number {
  return Number.isFinite(value) ? value / NANOSECONDS_PER_SECOND : 0;
}

export function durationToMinutes(value: number): number {
  return durationToSeconds(value) / SECONDS_PER_MINUTE;
}

export function minutesToDuration(value: number): number {
  return Math.round(value * SECONDS_PER_MINUTE * NANOSECONDS_PER_SECOND);
}

export function adaptDashboard(raw: DashboardStatsRaw): DashboardStats {
  const { avg_response_time, ...stats } = raw;
  return { ...stats, avgResponseSeconds: durationToSeconds(avg_response_time) };
}

export function adaptAccountRating(raw: AccountRatingRaw): AccountRating {
  const { avg_response_time, ...rating } = raw;
  return { ...rating, avgResponseSeconds: durationToSeconds(avg_response_time) };
}

export function adaptCompany(raw: CompanyRaw): Company {
  const { no_reply_threshold, ...company } = raw;
  return { ...company, noReplyThresholdMinutes: durationToMinutes(no_reply_threshold) };
}

export function asList<T>(value: T[] | null | undefined): T[] {
  return value ?? [];
}
