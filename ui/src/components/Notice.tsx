import React from 'react';
import { cardStyle, useTokens } from '../theme/tokens';
import { ErrorIcon } from './icons';

// Notice is the neutral card for loading and empty states.
export const Notice: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const t = useTokens();
  return (
    <div style={{ ...cardStyle(t), padding: '20px 24px', fontSize: '13px', color: t.textMuted }}>
      {children}
    </div>
  );
};

// Callout is the banner above a view's figures: an icon, a title, the message and a quieter note.
export const Callout: React.FC<{
  icon: React.ReactNode;
  title: string;
  note: React.ReactNode;
  children: React.ReactNode;
}> = ({ icon, title, note, children }) => {
  const t = useTokens();
  return (
    <div
      role="status"
      style={{
        ...cardStyle(t),
        display: 'flex',
        alignItems: 'flex-start',
        gap: '12px',
        marginBottom: '16px',
        padding: '14px 24px',
        color: t.text,
        fontSize: '13px',
      }}
    >
      <span style={{ display: 'flex', marginTop: '2px' }}>{icon}</span>
      <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
        <span style={{ color: t.textStrong, fontWeight: 500 }}>{title}</span>
        <span>{children}</span>
        <span style={{ color: t.textMuted, fontSize: '12px', overflowWrap: 'anywhere' }}>{note}</span>
      </div>
    </div>
  );
};

export const ErrorNotice: React.FC<{ message: string }> = ({ message }) => {
  const t = useTokens();
  return (
    <div
      role="alert"
      style={{
        ...cardStyle(t),
        padding: '16px 24px',
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        fontSize: '13px',
        color: t.text,
      }}
    >
      <ErrorIcon color={t.danger} />
      <span>Couldn&apos;t load cost data: {message}</span>
    </div>
  );
};
