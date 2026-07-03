import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'ParkPulse — Smart Parking Monitoring',
  description: 'Real-time parking gate monitoring: latency & ghost openings',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="uz">
      <body className="min-h-screen antialiased">{children}</body>
    </html>
  );
}
