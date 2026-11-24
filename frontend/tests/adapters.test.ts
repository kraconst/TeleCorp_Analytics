import { describe, expect, it } from 'vitest';
import { adaptCompany, adaptDashboard, durationToMinutes, durationToSeconds, minutesToDuration } from '@/api/adapters';

describe('Go duration adapters', () => {
  it('converts dashboard nanoseconds to seconds', () => {
    const result = adaptDashboard({
      messages_in: 10,
      messages_out: 8,
      active_chats: 4,
      new_contacts: 2,
      avg_response_time: 240_000_000_000,
      unanswered_chats: 1,
    });
    expect(result.avgResponseSeconds).toBe(240);
    expect(result).not.toHaveProperty('avg_response_time');
  });

  it('converts company threshold to minutes and back', () => {
    expect(durationToSeconds(7_200_000_000_000)).toBe(7200);
    expect(durationToMinutes(7_200_000_000_000)).toBe(120);
    expect(minutesToDuration(120)).toBe(7_200_000_000_000);
    expect(adaptCompany({ id: 1, name: 'Test', timezone: 'Europe/Moscow', no_reply_threshold: 7_200_000_000_000, created_at: '' }).noReplyThresholdMinutes).toBe(120);
  });

  it('does not turn invalid durations into huge UI values', () => {
    expect(durationToSeconds(Number.NaN)).toBe(0);
  });
});
