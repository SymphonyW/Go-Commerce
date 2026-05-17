import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter as Router, Route, Routes } from 'react-router-dom';
import './index.css';
import Navbar from './components/Navbar';
import Cart from './pages/Cart';
import Home from './pages/Home';
import Login from './pages/Login';
import MerchantCreate from './pages/MerchantCreate';
import MerchantDashboard from './pages/MerchantDashboard';
import MerchantList from './pages/MerchantList';
import MerchantOrders from './pages/MerchantOrders';
import MerchantProducts from './pages/MerchantProducts';
import OrderDetail from './pages/OrderDetail';
import Orders from './pages/Orders';
import ProductDetail from './pages/ProductDetail';
import Products from './pages/Products';
import Register from './pages/Register';

function App() {
  return (
    <Router>
      <div className="app">
        <Navbar />
        <div className="content">
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/register" element={<Register />} />
            <Route path="/login" element={<Login />} />
            <Route path="/products" element={<Products />} />
            <Route path="/products/:id" element={<ProductDetail />} />
            <Route path="/cart" element={<Cart />} />
            <Route path="/merchants" element={<MerchantList />} />
            <Route path="/merchants/create" element={<MerchantCreate />} />
            <Route path="/merchants/:id/products" element={<MerchantProducts />} />
            <Route path="/merchant" element={<MerchantDashboard />} />
            <Route path="/merchant/products" element={<MerchantProducts />} />
            <Route path="/merchant/orders" element={<MerchantOrders />} />
            <Route path="/orders" element={<Orders />} />
            <Route path="/orders/:id" element={<OrderDetail />} />
          </Routes>
        </div>
      </div>
    </Router>
  );
}

ReactDOM.createRoot(document.getElementById('app')).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
