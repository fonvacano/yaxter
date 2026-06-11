import { useState, type FormEvent, type ChangeEvent } from 'react';
import { useCreateTweet, uploadImage } from '../../api/hooks';

const MAX = 280;

export function Composer() {
  const [text, setText] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const create = useCreateTweet();

  const overLimit = text.length > MAX;
  const canPost = (text.trim().length > 0 || !!file) && !overLimit && !uploading && !create.isPending;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      let mediaIds: string[] | undefined;
      if (file) {
        setUploading(true);
        const media = await uploadImage(file);
        mediaIds = [String(media.id)];
        setUploading(false);
      }
      await create.mutateAsync({ text, media_ids: mediaIds, retweet_of_id: undefined });
      setText('');
      setFile(null);
    } catch (err) {
      setUploading(false);
      setError((err as Error).message);
    }
  }

  function onPick(e: ChangeEvent<HTMLInputElement>) {
    setFile(e.target.files?.[0] ?? null);
  }

  return (
    <form onSubmit={onSubmit} aria-label="compose">
      <label>
        What&apos;s happening?
        <textarea value={text} onChange={(e) => setText(e.target.value)} maxLength={MAX + 20} />
      </label>
      <input type="file" accept="image/jpeg,image/png,image/webp" onChange={onPick} aria-label="attach image" />
      <span aria-label="char count">{MAX - text.length}</span>
      {error && <p role="alert">{error}</p>}
      <button type="submit" disabled={!canPost}>
        {uploading ? 'Uploading…' : create.isPending ? 'Posting…' : 'Post'}
      </button>
    </form>
  );
}
