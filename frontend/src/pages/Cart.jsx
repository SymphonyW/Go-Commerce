import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import EmptyState from '../components/EmptyState';
import ErrorState from '../components/ErrorState';
import LoadingState from '../components/LoadingState';
import PageHeader from '../components/PageHeader';
import SectionCard from '../components/SectionCard';
import { cartAPI, getAPIErrorMessage, orderAPI } from '../services/api';
import { formatMoney, getProductImageUrl } from '../utils/display';

const Cart = () => {
  const [cart, setCart] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [updating, setUpdating] = useState(false);
  const [feedback, setFeedback] = useState(null);
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
        setCart(data?.items ? data : { items: [], total_amount_cents: 0 });
      } catch (fetchError) {
        console.error('Failed to fetch cart:', fetchError);
        setError('获取购物车失败');
      } finally {
        setLoading(false);
      }
    };

    fetchCart();
  }, [navigate]);

  const itemCount = useMemo(
    () => (cart?.items || []).reduce((sum, item) => sum + item.quantity, 0),
    [cart],
  );

  const refreshCart = async () => {
    const data = await cartAPI.getCart();
    setCart(data);
  };

  const handleUpdateQuantity = async (productId, quantity) => {
    if (quantity <= 0) {
      handleDeleteItem(productId);
      return;
    }

    try {
      setUpdating(true);
      setFeedback(null);
      await cartAPI.updateItem({ product_id: productId, quantity });
      await refreshCart();
    } catch (actionError) {
      console.error('Failed to update cart item:', actionError);
      setFeedback({ type: 'error', text: '更新购物车商品失败。' });
    } finally {
      setUpdating(false);
    }
  };

  const handleDeleteItem = async (productId) => {
    try {
      setUpdating(true);
      setFeedback(null);
      await cartAPI.deleteItem({ product_id: productId });
      await refreshCart();
    } catch (actionError) {
      console.error('Failed to delete cart item:', actionError);
      setFeedback({ type: 'error', text: '删除购物车商品失败。' });
    } finally {
      setUpdating(false);
    }
  };

  const handleClearCart = async () => {
    if (!window.confirm('确定要清空购物车吗？')) return;

    try {
      setUpdating(true);
      setFeedback(null);
      await cartAPI.clearCart();
      setCart({ items: [], total_amount_cents: 0 });
      setFeedback({ type: 'success', text: '购物车已清空。' });
    } catch (actionError) {
      console.error('Failed to clear cart:', actionError);
      setFeedback({ type: 'error', text: '清空购物车失败。' });
    } finally {
      setUpdating(false);
    }
  };

  const handleCheckout = async () => {
    if (!cart || cart.items.length === 0) {
      setFeedback({ type: 'error', text: '购物车为空，暂时无法结算。' });
      return;
    }

    try {
      setUpdating(true);
      setFeedback(null);
      const orderItems = cart.items.map((item) => ({
        product_id: item.product_id,
        quantity: item.quantity,
      }));
      await orderAPI.createOrder({ items: orderItems });
      await cartAPI.clearCart();
      navigate('/orders');
    } catch (actionError) {
      console.error('Failed to create order:', actionError);
      setFeedback({
        type: 'error',
        text: getAPIErrorMessage(actionError, '\u521b\u5efa\u8ba2\u5355\u5931\u8d25\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5\u3002'),
      });
    } finally {
      setUpdating(false);
    }
  };

  if (loading) {
    return <LoadingState label="正在加载购物车..." />;
  }

  if (error) {
    return <ErrorState title="购物车暂时不可用" description={error} />;
  }

  return (
    <div className="cart-page">
      <PageHeader
        eyebrow="Cart"
        title="我的购物车"
        subtitle="核对商品、数量与金额后继续结算。"
        meta={`${itemCount} 件商品`}
      />

      {feedback && <div className={`notice notice-${feedback.type}`}>{feedback.text}</div>}

      {!cart || cart.items.length === 0 ? (
        <EmptyState
          title="购物车还是空的"
          description="先去挑几件商品，回来后这里会自动汇总金额。"
          icon="🛒"
          action={
            <Link to="/products" className="btn btn-primary">
              去购物
            </Link>
          }
        />
      ) : (
        <div className="cart-layout">
          <div className="cart-items">
            {cart.items.map((item) => (
              <article key={item.product_id} className="cart-item">
                <div className="cart-item-image">
                  <img src={getProductImageUrl(item.image_url)} alt={item.product_name} />
                </div>
                <div className="cart-item-info">
                  <h3>{item.product_name}</h3>
                  <p>{formatMoney(item.price_cents)} / 件</p>
                </div>
                <div className="cart-item-quantity" aria-label={`${item.product_name} 数量`}>
                  <button
                    type="button"
                    onClick={() => handleUpdateQuantity(item.product_id, item.quantity - 1)}
                    disabled={updating}
                    className="quantity-btn"
                  >
                    −
                  </button>
                  <span className="quantity">{item.quantity}</span>
                  <button
                    type="button"
                    onClick={() => handleUpdateQuantity(item.product_id, item.quantity + 1)}
                    disabled={updating}
                    className="quantity-btn"
                  >
                    +
                  </button>
                </div>
                <div className="cart-item-total">{formatMoney(item.price_cents * item.quantity)}</div>
                <button
                  type="button"
                  onClick={() => handleDeleteItem(item.product_id)}
                  disabled={updating}
                  className="btn btn-danger btn-sm"
                >
                  删除
                </button>
              </article>
            ))}
          </div>

          <SectionCard className="cart-summary" title="订单摘要" subtitle="结算前再次确认金额。">
            <div className="cart-summary-row">
              <span>商品件数</span>
              <strong>{itemCount}</strong>
            </div>
            <div className="cart-summary-row cart-summary-total">
              <span>合计</span>
              <strong>{formatMoney(cart.total_amount_cents)}</strong>
            </div>
            <div className="cart-actions">
              <Link to="/products" className="btn btn-secondary">
                继续购物
              </Link>
              <button type="button" onClick={handleCheckout} disabled={updating} className="btn btn-primary">
                去结算
              </button>
              <button type="button" onClick={handleClearCart} disabled={updating} className="btn btn-ghost">
                清空购物车
              </button>
            </div>
          </SectionCard>
        </div>
      )}
    </div>
  );
};

export default Cart;
