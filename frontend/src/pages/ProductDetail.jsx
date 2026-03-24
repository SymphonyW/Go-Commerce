import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { productAPI, orderAPI } from '../services/api';

const ProductDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [product, setProduct] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [quantity, setQuantity] = useState(1);

  useEffect(() => {
    const fetchProduct = async () => {
      try {
        const data = await productAPI.getProduct(id);
        setProduct(data.product);
      } catch (error) {
        console.error('Failed to fetch product:', error);
        setError('获取商品详情失败');
      } finally {
        setLoading(false);
      }
    };

    fetchProduct();
  }, [id]);

  const handleAddToCart = () => {
    // 这里可以实现添加到购物车的逻辑
    alert('商品已添加到购物车');
  };

  const handleBuyNow = async () => {
    const token = localStorage.getItem('token');
    if (!token) {
      navigate('/login');
      return;
    }

    try {
      await orderAPI.createOrder({
        items: [{
          product_id: product.id,
          product_name: product.name,
          price: product.price,
          quantity: quantity,
        }],
      });
      alert('订单创建成功！');
      navigate('/orders');
    } catch (error) {
      console.error('Failed to create order:', error);
      alert('创建订单失败，请稍后重试');
    }
  };

  if (loading) {
    return <div className="loading">加载中...</div>;
  }

  if (error || !product) {
    return <div className="error-message">{error || '商品不存在'}</div>;
  }

  return (
    <div className="product-detail">
      <div className="product-detail-container">
        <div className="product-detail-image">
          <img 
            src={product.imageUrl || 'https://via.placeholder.com/400'} 
            alt={product.name} 
          />
        </div>
        <div className="product-detail-info">
          <h1>{product.name}</h1>
          <p className="product-detail-price">¥{product.price}</p>
          <p className="product-detail-stock">库存: {product.stock}</p>
          <p className="product-detail-category">分类: {product.category}</p>
          <div className="product-detail-description">
            <h3>商品描述</h3>
            <p>{product.description}</p>
          </div>
          <div className="product-detail-actions">
            <div className="quantity-control">
              <button 
                onClick={() => setQuantity(Math.max(1, quantity - 1))}
                className="quantity-btn"
              >
                -
              </button>
              <span className="quantity">{quantity}</span>
              <button 
                onClick={() => setQuantity(quantity + 1)}
                className="quantity-btn"
              >
                +
              </button>
            </div>
            <button className="btn btn-secondary" onClick={handleAddToCart}>
              添加到购物车
            </button>
            <button className="btn btn-primary" onClick={handleBuyNow}>
              立即购买
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ProductDetail;