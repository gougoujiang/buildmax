import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

export function MarkdownMessage({ content }) {
  return (
    <div className="page-chat__markdown">
      <Markdown remarkPlugins={[remarkGfm]}>{content}</Markdown>
    </div>
  );
}
