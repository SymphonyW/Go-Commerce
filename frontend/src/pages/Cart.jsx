import { useEffect, useState } from 'react';
import { cartAPI, orderAPI } from '../services/api';
import { Link, useNavigate } from 'react-router-dom';

const Cart = () => {
  const [cart, setCart] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [updating, setUpdating] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    const fetchCart = async () => {
      const token = localStorage.getItem('token');
      if (!token) {
        navigate('/login');
        return;
      }

      try {
        const data = await cartAPI.getCart();
        // 确保返回的数据包含items数组
        if (!data || !data.items) {
          setCart({ items: [], total_amount: 0 });
        } else {
          setCart(data);
        }
      } catch (error) {
        console.error('Failed to fetch cart:', error);
        setError('获取购物车失败');
      } finally {
        setLoading(false);
      }
    };

    fetchCart();
  }, [navigate]);

  const handleUpdateQuantity = async (productId, quantity) => {
    if (quantity <= 0) {
      handleDeleteItem(productId);
      return;
    }

    try {
      setUpdating(true);
      await cartAPI.updateItem({ product_id: productId, quantity });
      // 重新获取购物车数据
      const data = await cartAPI.getCart();
      setCart(data);
    } catch (error) {
      console.error('Failed to update cart item:', error);
      alert('更新购物车商品失败');
    } finally {
      setUpdating(false);
    }
  };

  const handleDeleteItem = async (productId) => {
    try {
      setUpdating(true);
      await cartAPI.deleteItem({ product_id: productId });
      // 重新获取购物车数据
      const data = await cartAPI.getCart();
      setCart(data);
    } catch (error) {
      console.error('Failed to delete cart item:', error);
      alert('删除购物车商品失败');
    } finally {
      setUpdating(false);
    }
  };

  const handleClearCart = async () => {
    if (window.confirm('确定要清空购物车吗？')) {
      try {
        setUpdating(true);
        await cartAPI.clearCart();
        setCart({ items: [], total_amount: 0 });
      } catch (error) {
        console.error('Failed to clear cart:', error);
        alert('清空购物车失败');
      } finally {
        setUpdating(false);
      }
    }
  };

  const handleCheckout = async () => {
    if (!cart || cart.items.length === 0) {
      alert('购物车为空');
      return;
    }

    try {
      // 从购物车创建订单
      const orderItems = cart.items.map(item => ({
        product_id: item.product_id,
        product_name: item.product_name,
        price: item.price,
        quantity: item.quantity
      }));

      await orderAPI.createOrder({ items: orderItems });
      // 清空购物车
      await cartAPI.clearCart();
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

  if (error) {
    return <div className="error-message">{error}</div>;
  }

  return (
    <div className="cart-container">
      <h1>我的购物车</h1>
      {error && <div className="error-message">{error}</div>}
      
      {!cart || cart.items.length === 0 ? (
        <div className="empty-cart">
          <p>购物车为空</p>
          <Link to="/products" className="btn">
            去购物
          </Link>
        </div>
      ) : (
        <>
          <div className="cart-items">
            {cart.items.map((item) => (
              <div key={item.product_id} className="cart-item">
                <div className="cart-item-image">
                  <img 
                    src={item.image_url || 'https://via.placeholder.com/100'} 
                    alt={item.product_name} 
                  />
                </div>
                <div className="cart-item-info">
                  <h3>{item.product_name}</h3>
                  <p className="cart-item-price">¥{item.price}</p>
                </div>
                <div className="cart-item-quantity">
                  <button 
                    onClick={() => handleUpdateQuantity(item.product_id, item.quantity - 1)}
                    disabled={updating}
                    className="quantity-btn"
                  >
                    -
                  </button>
                  <span className="quantity">{item.quantity}</span>
                  <button 
                    onClick={() => handleUpdateQuantity(item.product_id, item.quantity + 1)}
                    disabled={updating}
                    className="quantity-btn"
                  >
                    +
                  </button>
                </div>
                <div className="cart-item-total">
                  ¥{(item.price * item.quantity).toFixed(2)}
                </div>
                <div className="cart-item-actions">
                  <button 
                    onClick={() => handleDeleteItem(item.product_id)}
                    disabled={updating}
                    className="btn btn-danger btn-sm"
                  >
                    删除
                  </button>
                </div>
              </div>
            ))}
          </div>
          
          <div className="cart-summary">
            <div className="cart-total">
              <h3>合计：¥{cart.total_amount.toFixed(2)}</h3>
            </div>
            <div className="cart-actions">
              <button 
                onClick={handleClearCart}
                disabled={updating}
                className="btn btn-secondary"
              >
                清空购物车
              </button>
              <button 
                onClick={handleCheckout}
                disabled={updating}
                className="btn btn-primary"
              >
                去结算
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
};

export default Cart;