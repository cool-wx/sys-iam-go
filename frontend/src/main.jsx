import '@ant-design/v5-patch-for-react-19' // 首行：antd5 × React19 静态方法失效官方补丁（必须先于一切 antd 使用）
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App.jsx'

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
