import { useMutation, useQueryClient } from '@tanstack/react-query';
import { updateChatStatus } from '@/api/chats';
import { queryKeys } from '@/api/queryKeys';
import { useToast } from '@/components/feedback/ToastProvider';
import type { Chat, ChatDetails, ChatStatus } from '@/types/api';

export function useUpdateChatStatus(companyId: number) {
  const queryClient = useQueryClient();
  const { showToast } = useToast();

  return useMutation({
    mutationFn: ({ chatId, status }: { chatId: number; status: ChatStatus }) =>
      updateChatStatus(companyId, chatId, status),
    onMutate: async ({ chatId, status }) => {
      await Promise.all([
        queryClient.cancelQueries({ queryKey: queryKeys.chatLists(companyId) }),
        queryClient.cancelQueries({ queryKey: queryKeys.chat(companyId, chatId) }),
      ]);
      const previousLists = queryClient.getQueriesData<Chat[]>({ queryKey: queryKeys.chatLists(companyId) });
      const previousChat = queryClient.getQueryData<ChatDetails>(queryKeys.chat(companyId, chatId));
      queryClient.setQueriesData<Chat[]>({ queryKey: queryKeys.chatLists(companyId) }, (old) =>
        old?.map((chat) => chat.id === chatId ? { ...chat, status, status_source: 'manual' } : chat),
      );
      if (previousChat) {
        queryClient.setQueryData<ChatDetails>(queryKeys.chat(companyId, chatId), {
          ...previousChat,
          status,
          status_source: 'manual',
        });
      }
      return { previousLists, previousChat };
    },
    onError: (error, { chatId }, context) => {
      context?.previousLists.forEach(([key, data]) => queryClient.setQueryData(key, data));
      if (context?.previousChat) queryClient.setQueryData(queryKeys.chat(companyId, chatId), context.previousChat);
      showToast(`Статус не изменён: ${error.message}`, 'error');
    },
    onSuccess: () => showToast('Статус чата обновлён'),
    onSettled: (_data, _error, { chatId }) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.chatLists(companyId) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.chat(companyId, chatId) });
    },
  });
}
