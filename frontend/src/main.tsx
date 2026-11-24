import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { App } from './App';
import { DevSessionProvider } from './context/DevSessionContext';
import { NowProvider } from './context/NowContext';
import { ResolvedAlertsProvider } from './context/ResolvedAlertsContext';
import { ToastProvider } from './components/feedback/ToastProvider';
import './index.css';

function Providers() {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: {
      queries: { staleTime: 15_000, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
  }));

  return (
    <QueryClientProvider client={queryClient}>
      <DevSessionProvider>
        <ResolvedAlertsProvider>
          <ToastProvider>
            <NowProvider>
              <BrowserRouter>
                <App />
              </BrowserRouter>
            </NowProvider>
          </ToastProvider>
        </ResolvedAlertsProvider>
      </DevSessionProvider>
    </QueryClientProvider>
  );
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Providers />
  </StrictMode>,
);
