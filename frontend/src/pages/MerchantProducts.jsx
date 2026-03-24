import React, { useState, useEffect } from 'react';
import { merchantAPI, productAPI } from '../services/api';
import { useParams, useNavigate } from 'react-router-dom';

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

const MerchantProducts = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [products, setProducts] = useState([]);
  const [newProduct, setNewProduct] = useState({
    merchant_id: id,
    name: '',
    description: '',
    price: '',
    stock: '',
    category: '',
    image_url: ''
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchProducts();
  }, [id]);

  const fetchProducts = async () => {
    try {
      setLoading(true);
      const data = await productAPI.listProducts();
      // 过滤出当前商户的产品
      const merchantProducts = data.products.filter(product => product.merchant_id == id);
      setProducts(merchantProducts);
    } catch (err) {
      setError('获取产品列表失败');
      console.error('Error fetching products:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setNewProduct(prev => ({ ...prev, [name]: value }));
  };

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
        merchant_id: id,
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

  const handleDeleteProduct = async (productId) => {
    try {
      setLoading(true);
      await merchantAPI.deleteProduct({ merchant_id: parseInt(id), product_id: productId });
      await fetchProducts();
    } catch (err) {
      setError('删除产品失败');
      console.error('Error deleting product:', err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="merchant-products-container">
      <h1>商户产品管理</h1>
      
      {error && <div className="error-message">{error}</div>}
      
      <div className="page-actions">
        <button type="button" onClick={() => navigate('/merchants')} className="btn btn-secondary">
          返回商户列表
        </button>
      </div>
      
      {/* 添加产品表单 */}
      <div className="add-product">
        <h2>添加产品</h2>
        <form onSubmit={handleAddProduct} className="product-form">
          <div className="form-row">
            <div className="form-group">
              <label htmlFor="merchant_id">商户ID</label>
              <input
                type="number"
                id="merchant_id"
                name="merchant_id"
                value={newProduct.merchant_id}
                onChange={handleChange}
                readOnly
              />
            </div>
            <div className="form-group">
              <label htmlFor="name">产品名称</label>
              <input
                type="text"
                id="name"
                name="name"
                value={newProduct.name}
                onChange={handleChange}
                required
              />
            </div>
          </div>
          <div className="form-row">
            <div className="form-group">
              <label htmlFor="price">价格</label>
              <input
                type="number"
                step="0.01"
                id="price"
                name="price"
                value={newProduct.price}
                onChange={handleChange}
                required
              />
            </div>
            <div className="form-group">
              <label htmlFor="stock">库存</label>
              <input
                type="number"
                id="stock"
                name="stock"
                value={newProduct.stock}
                onChange={handleChange}
                required
              />
            </div>
          </div>
          <div className="form-row">
            <div className="form-group">
              <label htmlFor="category">分类</label>
              <input
                type="text"
                id="category"
                name="category"
                value={newProduct.category}
                onChange={handleChange}
                required
              />
            </div>
            <div className="form-group">
              <label htmlFor="image_url">图片URL</label>
              <input
                type="text"
                id="image_url"
                name="image_url"
                value={newProduct.image_url}
                onChange={handleChange}
                required
              />
            </div>
          </div>
          <div className="form-group">
            <label htmlFor="description">产品描述</label>
            <textarea
              id="description"
              name="description"
              value={newProduct.description}
              onChange={handleChange}
              required
            />
          </div>
          <div className="form-actions">
            <button type="submit" disabled={loading} className="btn btn-primary">
              {loading ? '添加中...' : '添加产品'}
            </button>
          </div>
        </form>
      </div>
      
      {/* 产品列表 */}
      <div className="products-list">
        <h2>产品列表</h2>
        {loading ? (
          <p>加载中...</p>
        ) : (
          <div className="products-grid">
            {products.map((product) => (
              <div key={product.id} className="product-card">
                <img 
                  src={processImageUrl(product.image_url)} 
                  alt={product.name} 
                  className="product-image"
                />
                <h3>{product.name}</h3>
                <p>描述: {product.description}</p>
                <p>价格: ¥{product.price}</p>
                <p>库存: {product.stock}</p>
                <p>分类: {product.category}</p>
                <div className="card-actions">
                  <button
                    onClick={() => handleDeleteProduct(product.id)}
                    disabled={loading}
                    className="btn btn-danger"
                  >
                    删除
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default MerchantProducts;