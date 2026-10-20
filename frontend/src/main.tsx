import '@fontsource/fira-sans-extra-condensed/cyrillic-800.css';
import '@fontsource/fira-sans-extra-condensed/latin-800.css';
import '@fontsource/onest/cyrillic-400.css';
import '@fontsource/onest/latin-400.css';
import '@fontsource/onest/cyrillic-600.css';
import '@fontsource/onest/latin-600.css';
import '@fontsource/onest/cyrillic-700.css';
import '@fontsource/onest/latin-700.css';
import './styles/global.css';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './app/App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
