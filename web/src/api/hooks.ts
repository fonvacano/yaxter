import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from '@tanstack/react-query';
import {
  createMedia,
  completeMedia,
  createTweet,
  deleteTweet,
  followUser,
  getHomeTimeline,
  getMedia,
  getUnreadCount,
  getUser,
  getUserTweets,
  likeTweet,
  listNotifications,
  markNotificationsRead,
  unlikeTweet,
  type CreateTweetRequest,
  type MarkReadRequest,
  type Notification,
  type Tweet,
  type TweetPage,
  type NotificationPage,
  type CreateMediaRequestContentType,
} from './generated';

// ---------------------------------------------------------------------------
// body<T>() — bridges the runtime/declared type mismatch.
// The orval fetcher returns `{ data, status, headers }` at runtime but the
// declared return types are union-of-those-shapes.  We extract `.data` when
// present, falling back to the value itself.
// ---------------------------------------------------------------------------
export function body<T>(r: unknown): T {
  if (r !== null && typeof r === 'object' && 'data' in (r as object)) {
    return (r as { data: T }).data;
  }
  return r as T;
}

// ---------------------------------------------------------------------------
// Query key factories
// ---------------------------------------------------------------------------
export const timelineKeys = {
  home: () => ['timeline', 'home'] as const,
  profile: (username: string) => ['timeline', 'profile', username] as const,
};

export const notifKeys = {
  all: () => ['notifications'] as const,
  list: () => ['notifications', 'list'] as const,
  unread: () => ['notifications', 'unread'] as const,
};

// ---------------------------------------------------------------------------
// Timeline hooks
// ---------------------------------------------------------------------------

export function useHomeTimeline() {
  const q = useInfiniteQuery<TweetPage, Error, InfiniteData<TweetPage>, readonly string[], string | undefined>({
    queryKey: timelineKeys.home(),
    queryFn: ({ pageParam }) =>
      body<TweetPage>(getHomeTimeline({ cursor: pageParam })),
    initialPageParam: undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  });

  const tweets: Tweet[] = q.data?.pages.flatMap((p) => p.items) ?? [];
  return { ...q, tweets };
}

export function useProfileTimeline(username: string) {
  const q = useInfiniteQuery<TweetPage, Error, InfiniteData<TweetPage>, readonly string[], string | undefined>({
    queryKey: timelineKeys.profile(username),
    queryFn: ({ pageParam }) =>
      body<TweetPage>(getUserTweets(username, { cursor: pageParam })),
    initialPageParam: undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  });

  const tweets: Tweet[] = q.data?.pages.flatMap((p) => p.items) ?? [];
  return { ...q, tweets };
}

// ---------------------------------------------------------------------------
// Tweet mutations
// ---------------------------------------------------------------------------

export function useCreateTweet() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (req: CreateTweetRequest) =>
      body<Tweet>(await createTweet(req)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['timeline'] }),
  });
}

export function useDeleteTweet() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => body<void>(await deleteTweet(id)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['timeline'] }),
  });
}

export function useSetLike() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, like }: { id: string; like: boolean }) =>
      body<void>(like ? await likeTweet(id) : await unlikeTweet(id)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['timeline'] }),
  });
}

// ---------------------------------------------------------------------------
// User hooks
// ---------------------------------------------------------------------------

export function useUser(username: string) {
  return useQuery({
    queryKey: ['user', username],
    queryFn: () => body<import('./generated').User>(getUser(username)),
    enabled: !!username,
  });
}

export function useFollow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (username: string) => body<void>(await followUser(username)),
    onSuccess: (_data, username) =>
      qc.invalidateQueries({ queryKey: ['user', username] }),
  });
}

// ---------------------------------------------------------------------------
// Notification hooks
// ---------------------------------------------------------------------------

export function useUnreadCount() {
  return useQuery({
    queryKey: notifKeys.unread(),
    queryFn: () =>
      body<import('./generated').UnreadCount>(getUnreadCount()),
    refetchInterval: 30_000,
  });
}

export function useNotifications() {
  const q = useInfiniteQuery<NotificationPage, Error, InfiniteData<NotificationPage>, readonly string[], string | undefined>({
    queryKey: notifKeys.list(),
    queryFn: ({ pageParam }) =>
      body<NotificationPage>(listNotifications({ cursor: pageParam })),
    initialPageParam: undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
  });

  const items: Notification[] = q.data?.pages.flatMap((p) => p.items) ?? [];
  return { ...q, items };
}

export function useMarkRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (req: MarkReadRequest) =>
      body<void>(await markNotificationsRead(req)),
    onSuccess: () => qc.invalidateQueries({ queryKey: notifKeys.all() }),
  });
}

// ---------------------------------------------------------------------------
// Media upload
// ---------------------------------------------------------------------------

export async function uploadImage(file: File): Promise<import('./generated').Media> {
  const ticket = body<import('./generated').MediaUploadTicket>(
    await createMedia({
      content_type: file.type as CreateMediaRequestContentType,
      size_bytes: file.size,
    }),
  );

  await fetch(ticket.upload_url, {
    method: 'PUT',
    headers: { 'Content-Type': file.type },
    body: file,
  });

  await body<import('./generated').Media>(completeMedia(ticket.media_id));

  for (let i = 0; i < 10; i++) {
    const media = body<import('./generated').Media>(await getMedia(ticket.media_id));
    if (media.status === 'ready') return media;
    if (media.status === 'failed') throw new Error('Media processing failed');
    await new Promise((r) => setTimeout(r, 1000));
  }

  throw new Error('Media upload timed out');
}

// ---------------------------------------------------------------------------
// Optimistic like (appended last per spec)
// ---------------------------------------------------------------------------
export function useOptimisticLike() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, like }: { id: string; like: boolean }) =>
      body<void>(like ? await likeTweet(id) : await unlikeTweet(id)),
    onSettled: () => qc.invalidateQueries({ queryKey: ['timeline'] }),
  });
}
