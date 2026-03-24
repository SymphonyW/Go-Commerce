import React, { useState, useEffect } from 'react';
import { merchantAPI, productAPI } from '../services/api';

// 处理图片URL，将维基百科页面URL转换为实际图片文件URL
const processImageUrl = (url) => {
  if (!url) return 'https://via.placeholder.com/100';
  
  // 检查是否是维基百科的图片页面URL
  if (url.includes('wikipedia.org/wiki/File:')) {
    // 提取文件名
    const fileName = url.split('/').pop();
    // 构建实际的图片文件URL
    return `https://upload.wikimedia.org/wikipedia/commons/thumb/${fileName.charAt(0)}/${fileName.charAt(0) + fileName.charAt(1)}/${fileName}/100px-${fileName}`;
  }
  
  return url;
};

const Merchants = () => {
  // 状态管理
  const [merchants, setMerchants] = useState([]);
  const [products, setProducts] = useState([]);
  const [newMerchant, setNewMerchant] = useState({ name: '', contact_info: '' });
  const [newProduct, setNewProduct] = useState({
    merchant_id: '',
    name: '',
    description: '',
    price: '',
    stock: '',
    category: '',
    image_url: ''
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  // 加载商户列表
  useEffect(() => {
    fetchMerchants();
    fetchProducts();
  }, []);

  // 获取商户列表
  const fetchMerchants = async () => {
    try {
      setLoading(true);
      const data = await merchantAPI.listMerchants();
      setMerchants(data.merchants || []);
    } catch (err) {
      setError('获取商户列表失败');
      console.error('Error fetching merchants:', err);
    } finally {
      setLoading(false);
    }
  };

  // 获取产品列表
  const fetchProducts = async () => {
    try {
      const data = await productAPI.listProducts();
      setProducts(data.products || []);
    } catch (err) {
      console.error('Error fetching products:', err);
    }
  };

  // 创建商户
  const handleCreateMerchant = async (e) => {
    e.preventDefault();
    try {
      setLoading(true);
      await merchantAPI.createMerchant(newMerchant);
      setNewMerchant({ name: '', contact_info: '' });
      await fetchMerchants();
    } catch (err) {
      setError('创建商户失败');
      console.error('Error creating merchant:', err);
    } finally {
      setLoading(false);
    }
  };

  // 商户添加产品
  const handleAddProduct = async (e) => {
    e.preventDefault();
    try {
      setLoading(true);
      // 转换类型
      const productData = {
        ...newProduct,
        merchant_id: parseInt(newProduct.merchant_id),
        price: parseFloat(newProduct.price),
        stock: parseInt(newProduct.stock)
      };
      await merchantAPI.addProduct(productData);
      setNewProduct({
        merchant_id: '',
        name: '',
        description: '',
        price: '',
        stock: '',
        category: '',
        image_url: ''
      });
      await fetchProducts();
    } catch (err) {
      setError('添加产品失败');
      console.error('Error adding product:', err);
    } finally {
      setLoading(false);
    }
  };

  // 商户删除产品
  const handleDeleteProduct = async (merchantId, productId) => {
    try {
      setLoading(true);
      await merchantAPI.deleteProduct({ merchant_id: merchantId, product_id: productId });
      await fetchProducts();
    } catch (err) {
      setError('删除产品失败');
      console.error('Error deleting product:', err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="merchants-container">
      <h1>商户管理</h1>
      
      {error && <div className="error-message">{error}</div>}
      
      {/* 创建商户表单 */}
      <div className="create-merchant">
        <h2>创建新商户</h2>
        <form onSubmit={handleCreateMerchant}>
          <div className="form-group">
            <label>商户名称</label>
            <input
              type="text"
              value={newMerchant.name}
              onChange={(e) => setNewMerchant({ ...newMerchant, name: e.target.value })}
              required
            />
          </div>
          <div className="form-group">
            <label>联系信息</label>
            <input
              type="text"
              value={newMerchant.contact_info}
              onChange={(e) => setNewMerchant({ ...newMerchant, contact_info: e.target.value })}
              required
            />
          </div>
          <button type="submit" disabled={loading}>
            {loading ? '创建中...' : '创建商户'}
          </button>
        </form>
      </div>
      
      {/* 商户列表 */}
      <div className="merchants-list">
        <h2>商户列表</h2>
        {loading ? (
          <p>加载中...</p>
        ) : (
          <ul>
            {merchants.map((merchant) => (
              <li key={merchant.id}>
                <h3>{merchant.name} (ID: {merchant.id})</h3>
                <p>联系信息: {merchant.contact_info}</p>
                <p>创建时间: {new Date(merchant.created_at).toLocaleString()}</p>
              </li>
            ))}
          </ul>
        )}
      </div>
      
      {/* 添加产品表单 */}
      <div className="add-product">
        <h2>添加产品</h2>
        <form onSubmit={handleAddProduct}>
          <div className="form-group">
            <label>商户ID</label>
            <input
              type="number"
              value={newProduct.merchant_id}
              onChange={(e) => setNewProduct({ ...newProduct, merchant_id: e.target.value })}
              required
            />
          </div>
          <div className="form-group">
            <label>产品名称</label>
            <input
              type="text"
              value={newProduct.name}
              onChange={(e) => setNewProduct({ ...newProduct, name: e.target.value })}
              required
            />
          </div>
          <div className="form-group">
            <label>产品描述</label>
            <textarea
              value={newProduct.description}
              onChange={(e) => setNewProduct({ ...newProduct, description: e.target.value })}
              required
            />
          </div>
          <div className="form-group">
            <label>价格</label>
            <input
              type="number"
              step="0.01"
              value={newProduct.price}
              onChange={(e) => setNewProduct({ ...newProduct, price: e.target.value })}
              required
            />
          </div>
          <div className="form-group">
            <label>库存</label>
            <input
              type="number"
              value={newProduct.stock}
              onChange={(e) => setNewProduct({ ...newProduct, stock: e.target.value })}
              required
            />
          </div>
          <div className="form-group">
            <label>分类</label>
            <input
              type="text"
              value={newProduct.category}
              onChange={(e) => setNewProduct({ ...newProduct, category: e.target.value })}
              required
            />
          </div>
          <div className="form-group">
            <label>图片URL</label>
            <input
              type="text"
              value={newProduct.image_url}
              onChange={(e) => setNewProduct({ ...newProduct, image_url: e.target.value })}
              required
            />
          </div>
          <button type="submit" disabled={loading}>
            {loading ? '添加中...' : '添加产品'}
          </button>
        </form>
      </div>
      
      {/* 产品列表 */}
      <div className="products-list">
        <h2>产品列表</h2>
        {loading ? (
          <p>加载中...</p>
        ) : (
          <ul>
            {products.map((product) => (
              <li key={product.id}>
                <h3>{product.name}</h3>
                <img 
                  src={processImageUrl(product.imageUrl || product.image_url)} 
                  alt={product.name} 
                  style={{ width: '100px', height: '100px' }}
                />
                <p>描述: {product.description}</p>
                <p>价格: ¥{product.price}</p>
                <p>库存: {product.stock}</p>
                <p>分类: {product.category}</p>
                <p>商户ID: {product.merchant_id}</p>
                <button
                  onClick={() => handleDeleteProduct(product.merchant_id, product.id)}
                  disabled={loading}
                >
                  删除
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
};

export default Merchants;