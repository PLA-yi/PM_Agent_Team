import { useCallback, useEffect, useState } from "react";
import { SocialPost, getPosts } from "../lib/api";

interface Props {
  taskId?: string;
}

const PAGE = 50;

export function PostsViewer({ taskId }: Props) {
  const [posts, setPosts] = useState<SocialPost[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [platform, setPlatform] = useState("");
  const [q, setQ] = useState("");
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async (reset = false) => {
    if (!taskId) return;
    setLoading(true);
    setErr(null);
    try {
      const off = reset ? 0 : offset;
      const r = await getPosts(taskId, { platform, q, limit: PAGE, offset: off });
      setPosts(reset ? r.posts : [...posts, ...r.posts]);
      setTotal(r.total);
      setOffset(off + r.posts.length);
    } catch (e) {
      setErr(String(e));
    } finally {
      setLoading(false);
    }
  }, [taskId, platform, q, offset, posts]);

  // 任务变化或筛选变化时重置
  useEffect(() => {
    setPosts([]);
    setOffset(0);
    setTotal(0);
    if (taskId) {
      load(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId, platform]);

  if (!taskId) {
    return <div className="p-6 text-xs text-muted2 text-center mono">no task selected</div>;
  }

  // 推断可用 platforms（from loaded posts）
  const platformsSeen = Array.from(new Set(posts.map((p) => p.platform)));

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="px-4 py-2 border-b border-border bg-bg flex items-center gap-2 text-xs flex-wrap">
        <span className="font-semibold text-ink">Posts</span>
        <span className="ios-chip mono">{total} total</span>
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") load(true); }}
          placeholder="搜索 title/content/author"
          className="ios-input flex-1 max-w-xs py-1 text-xs"
        />
        <select
          value={platform}
          onChange={(e) => setPlatform(e.target.value)}
          className="ios-input py-1 text-xs w-28"
        >
          <option value="">all platforms</option>
          {platformsSeen.map((p) => (
            <option key={p} value={p}>{p}</option>
          ))}
          <option value="reddit">reddit</option>
          <option value="x">x</option>
          <option value="douyin">douyin</option>
          <option value="tiktok">tiktok</option>
          <option value="youtube">youtube</option>
        </select>
        <button onClick={() => load(true)} className="ios-btn ios-btn-ghost py-1 px-2 text-xs">
          搜索
        </button>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        {err && (
          <div className="p-3 m-3 text-xs text-danger border border-danger/30 rounded">{err}</div>
        )}
        {posts.length === 0 && !loading && (
          <div className="p-6 text-xs text-muted2 text-center mono">no posts</div>
        )}
        {posts.length > 0 && (
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-white border-b border-border">
              <tr className="text-muted2 uppercase tracking-wider">
                <th className="text-left px-3 py-2 font-semibold w-16">Plat</th>
                <th className="text-left px-3 py-2 font-semibold w-24">Author</th>
                <th className="text-left px-3 py-2 font-semibold">Title</th>
                <th className="text-right px-3 py-2 font-semibold w-12">↑</th>
                <th className="text-right px-3 py-2 font-semibold w-12">💬</th>
              </tr>
            </thead>
            <tbody>
              {posts.map((p) => (
                <tr key={p.platform + ":" + p.id} className="border-b border-border hover:bg-bg2/40">
                  <td className="px-3 py-2 mono text-muted2">{p.platform}</td>
                  <td className="px-3 py-2 mono truncate max-w-[120px]" title={p.author}>{p.author}</td>
                  <td className="px-3 py-2">
                    <a
                      href={p.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-link hover:underline"
                      title={p.content}
                    >
                      {p.title || "(no title)"}
                    </a>
                  </td>
                  <td className="px-3 py-2 text-right mono tabular-nums text-muted">
                    {p.engagement?.likes ?? 0}
                  </td>
                  <td className="px-3 py-2 text-right mono tabular-nums text-muted">
                    {p.engagement?.comments ?? 0}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Footer / load more */}
      {posts.length > 0 && posts.length < total && (
        <div className="border-t border-border px-3 py-2 bg-bg flex items-center justify-between text-xs">
          <span className="text-muted2 mono">{posts.length} / {total}</span>
          <button
            disabled={loading}
            onClick={() => load(false)}
            className="ios-btn ios-btn-ghost py-1 px-3 text-xs disabled:opacity-50"
          >
            {loading ? "loading..." : `load next ${Math.min(PAGE, total - posts.length)}`}
          </button>
        </div>
      )}
    </div>
  );
}
