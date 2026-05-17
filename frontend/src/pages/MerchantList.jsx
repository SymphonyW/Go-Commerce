import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { merchantAPI } from '../services/api';

const MerchantList = () => {
  const [merchants, setMerchants] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const role = localStorage.getItem('role');
  const userId = Number(localStorage.getItem('user_id'));
  const canManageMerchants = role === 'merchant' || role === 'admin';

  useEffect(() => {
    const fetchMerchants = async () => {
      try {
        setLoading(true);
        const data = await merchantAPI.listMerchants();
        setMerchants(data.merchants || []);
      } catch (err) {
        setError(err.response?.data?.error || '获取商户列表失败');
      } finally {
        setLoading(false);
      }
    };

    fetchMerchants();
  }, []);

  return (
    <div className="merchant-list-container">
      <h1>商户管理</h1>
      {error && <div className="error-message">{error}</div>}

      {canManageMerchants ? (
        <div className="page-actions">
          <Link to="/merchants/create" className="btn btn-primary">
            创建新商户
          </Link>
        </div>
      ) : (
        <div className="info-message">当前账户没有商家写权限，仅可查看商户信息。</div>
      )}

      <div className="merchants-list">
        <h2>商户列表</h2>
        {loading ? (
          <p>加载中...</p>
        ) : (
          <div className="merchants-grid">
            {merchants.map((merchant) => (
              <div key={merchant.id} className="merchant-card">
                <h3>{merchant.name}</h3>
                <p>商户 ID：{merchant.id}</p>
                <p>联系方式：{merchant.contact_info}</p>
                <p>创建时间：{new Date(merchant.created_at).toLocaleString()}</p>
                {(role === 'admin' || (role === 'merchant' && merchant.owner_user_id === userId)) && (
                  <div className="card-actions">
                    <Link to={`/merchant?merchant_id=${merchant.id}`} className="btn btn-sm">
                      进入后台
                    </Link>
                    <Link to={`/merchant/products?merchant_id=${merchant.id}`} className="btn btn-sm">
                      管理商品
                    </Link>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default MerchantList;
